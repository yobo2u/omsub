package cursorapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cursorplugin/internal/cursorproto"
)

type RunInput struct {
	AccessToken string
	Model       string
	System      string
	Prompt      string
	Tools       []cursorproto.ToolDefinition
	Images      []cursorproto.ImageAttachment
	Attachments []cursorproto.FileAttachment
}

func (client *Client) Run(ctx context.Context, input RunInput, emit func(cursorproto.ServerEvent) error) error {
	if input.AccessToken == "" || input.Model == "" || input.Prompt == "" {
		return errors.New("Cursor run requires token, model, and prompt")
	}
	requestID := randomUUID()
	sessionID := randomUUID()
	messageID := randomUUID()
	runPayload, err := cursorproto.EncodeRunRequest(cursorproto.RunRequest{
		ConversationID: "cursor_" + strings.ReplaceAll(randomUUID(), "-", ""),
		MessageID:      messageID,
		Model:          input.Model,
		System:         input.System,
		Prompt:         input.Prompt,
		TimeZone:       "UTC",
		Tools:          input.Tools,
		Images:         input.Images,
		Attachments:    input.Attachments,
	})
	if err != nil {
		return err
	}
	heartbeatPayload, err := cursorproto.EncodeClientHeartbeat()
	if err != nil {
		return err
	}
	runBodyReader, runBodyWriter := io.Pipe()
	runContext, cancelRun := context.WithCancelCause(ctx)
	defer cancelRun(nil)
	firstFrameTimer := time.AfterFunc(client.firstFrame, func() {
		cancelRun(ErrFirstFrameTimeout)
	})
	defer firstFrameTimer.Stop()
	request, err := http.NewRequestWithContext(runContext, http.MethodPost, client.endpoint("/agent.v1.AgentService/Run"), runBodyReader)
	if err != nil {
		return errors.Join(fmt.Errorf("create Cursor Run request: %w", err), runBodyReader.Close(), runBodyWriter.Close())
	}
	client.applyHeaders(request, requestHeaders{
		accessToken: input.AccessToken,
		requestID:   requestID,
		sessionID:   sessionID,
	}, "application/connect+proto")
	stopWriter := make(chan struct{})
	outbound := make(chan []byte, 16)
	writeResult := make(chan error, 1)
	go func() {
		writeResult <- writeRunFrames(runContext, runBodyWriter, runPayload, heartbeatPayload, client.heartbeat, outbound, stopWriter)
	}()
	runResponse, err := client.httpClient.Do(request)
	firstFrameTimer.Stop()
	if err != nil {
		readerErr := runBodyReader.CloseWithError(err)
		close(stopWriter)
		if errors.Is(context.Cause(runContext), ErrFirstFrameTimeout) {
			return errors.Join(ErrFirstFrameTimeout, <-writeResult, readerErr, runBodyWriter.CloseWithError(err))
		}
		return errors.Join(fmt.Errorf("start Cursor Run: %w", err), <-writeResult, readerErr, runBodyWriter.CloseWithError(err))
	}
	if runResponse.StatusCode != http.StatusOK {
		status := runResponse.StatusCode
		readerErr := runBodyReader.Close()
		close(stopWriter)
		return errors.Join(fmt.Errorf("Cursor Run returned HTTP %d", status), readerErr, <-writeResult, runResponse.Body.Close(), runBodyWriter.Close())
	}
	streamErr := readRunStream(runContext, runResponse.Body, emit, cursorproto.NewBlobStore(), outbound)
	readerErr := runBodyReader.Close()
	close(stopWriter)
	return errors.Join(streamErr, readerErr, <-writeResult, runResponse.Body.Close(), runBodyWriter.Close())
}

func writeRunFrames(
	ctx context.Context,
	writer io.Writer,
	runPayload []byte,
	heartbeatPayload []byte,
	heartbeatInterval time.Duration,
	outbound <-chan []byte,
	stop <-chan struct{},
) error {
	if _, err := writer.Write(cursorproto.EncodeConnectFrame(runPayload)); err != nil {
		select {
		case <-ctx.Done():
			return nil
		case <-stop:
			return nil
		default:
		}
		return fmt.Errorf("write Cursor Run request: %w", err)
	}
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-stop:
			return nil
		case payload := <-outbound:
			if _, err := writer.Write(cursorproto.EncodeConnectFrame(payload)); err != nil {
				return fmt.Errorf("write Cursor client reply: %w", err)
			}
		case <-ticker.C:
			if _, err := writer.Write(cursorproto.EncodeConnectFrame(heartbeatPayload)); err != nil {
				select {
				case <-ctx.Done():
					return nil
				case <-stop:
					return nil
				default:
				}
				return fmt.Errorf("write Cursor heartbeat: %w", err)
			}
		}
	}
}

func readRunStream(
	ctx context.Context,
	body io.Reader,
	emit func(cursorproto.ServerEvent) error,
	blobs *cursorproto.BlobStore,
	outbound chan<- []byte,
) error {
	decoder := cursorproto.NewFrameDecoder(32 << 20)
	buffer := make([]byte, 32<<10)
	for {
		count, readErr := body.Read(buffer)
		if count > 0 {
			frames, err := decoder.Push(buffer[:count])
			if err != nil {
				return err
			}
			for _, frame := range frames {
				if frame.Flags != 0 {
					return fmt.Errorf("Cursor stream ended with Connect flags %d", frame.Flags)
				}
				reply, handled, err := blobs.HandleServerMessage(frame.Payload)
				if err != nil {
					return err
				}
				if handled {
					select {
					case outbound <- reply:
					case <-ctx.Done():
						return context.Cause(ctx)
					}
				}
				event, err := cursorproto.DecodeServerEvent(frame.Payload)
				if err != nil {
					return err
				}
				if err := emit(event); err != nil {
					return fmt.Errorf("emit Cursor event: %w", err)
				}
				if event.Kind == cursorproto.EventDone {
					return nil
				}
			}
		}
		if errors.Is(readErr, io.EOF) {
			return errors.New("Cursor stream closed before turn end")
		}
		if readErr != nil {
			return fmt.Errorf("read Cursor stream: %w", readErr)
		}
	}
}

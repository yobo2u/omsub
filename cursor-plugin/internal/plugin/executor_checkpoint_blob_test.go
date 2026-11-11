package plugin

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"cursorplugin/internal/cursorapi"
	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
)

func Test_Checkpoint_suffix_answers_Cursor_blob_request_from_previous_turn(t *testing.T) {
	blob := []byte("blob-from-first-turn")
	blobID := checkpointTestBytesField(1, []byte("b"))
	checkpoint := []byte{0x0a, 0x01, 0x62}
	var calls atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if _, err := readCheckpointTestPayload(request.Body); err != nil {
			t.Error(err)
			return
		}
		turn := calls.Add(1)
		kv := []byte{0x08, 0x01}
		if turn == 1 {
			kv = append(kv, checkpointTestBytesField(3, append(blobID, checkpointTestBytesField(2, blob)...))...)
		} else {
			kv = append(kv, checkpointTestBytesField(2, blobID)...)
		}
		_, _ = writer.Write(cursorproto.EncodeConnectFrame(checkpointTestBytesField(4, kv)))
		writer.(http.Flusher).Flush()
		reply, err := readCheckpointTestPayload(request.Body)
		if err != nil {
			t.Error(err)
			return
		}
		if turn > 1 && !bytes.Contains(reply, blob) {
			failed := cursorproto.EncodeConnectFrame([]byte(`{"error":{"code":"failed_precondition"}}`))
			failed[0] = 2
			_, _ = writer.Write(failed)
			return
		}
		_, _ = writer.Write(cursorproto.EncodeConnectFrame(checkpointTestBytesField(1, checkpointTestBytesField(1, checkpointTestBytesField(1, []byte("answer"))))))
		_, _ = writer.Write(cursorproto.EncodeConnectFrame(checkpointTestBytesField(3, checkpoint)))
		_, _ = writer.Write(cursorproto.EncodeConnectFrame([]byte{0x0a, 0x02, 0x72, 0x00}))
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client, err := cursorapi.NewClient(cursorapi.Config{BaseURL: server.URL, HTTPClient: server.Client()})
	require.NoError(t, err)
	handler := NewHandler(Dependencies{Cursor: client})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	first := executorFixture(t, "session", "account", "auth", "auto", "", []map[string]any{
		textMessage("user", "seed"),
	})
	second := executorFixture(t, "session", "account", "auth", "auto", "", []map[string]any{
		textMessage("user", "seed"), textMessage("assistant", "answer"), textMessage("user", "next"),
	})

	_, err = handler.execute(ctx, first)
	require.NoError(t, err)
	_, err = handler.execute(ctx, second)
	require.NoError(t, err)
	require.EqualValues(t, 2, calls.Load())
}

func checkpointTestBytesField(number protowire.Number, value []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, number, protowire.BytesType), value)
}

func readCheckpointTestPayload(reader io.Reader) ([]byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(reader, header[:]); err != nil {
		return nil, err
	}
	payload := make([]byte, binary.BigEndian.Uint32(header[1:]))
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

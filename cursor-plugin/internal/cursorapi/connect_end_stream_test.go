package cursorapi

import (
	"context"
	"testing"

	"cursorplugin/internal/cursorproto"

	"github.com/stretchr/testify/require"
)

func Test_Client_Run_accepts_clean_connect_end_stream_after_turn_end(t *testing.T) {
	// Given
	endStream := connectFrame([]byte(`{}`))
	endStream[0] = 0x02
	body := newChunkBody(
		connectFrame(textServerMessage("complete")),
		connectFrame(turnEndedServerMessage()),
		endStream,
	)
	client := newTestClient(t, staticResponseTransport(body), Config{})
	var text string

	// When
	result, err := client.Run(context.Background(), validRunInput(), func(event cursorproto.ServerEvent) error {
		if event.Kind == cursorproto.EventText {
			text += event.Text
		}
		return nil
	})

	// Then
	require.NoError(t, err)
	require.Equal(t, "complete", text)
	require.True(t, result.OutputExposed)
}

func Test_Client_Run_maps_connect_end_stream_invalid_argument(t *testing.T) {
	// Given
	endStream := connectFrame([]byte(`{"error":{"code":"invalid_argument","message":"checkpoint rejected"}}`))
	endStream[0] = 0x02
	body := newChunkBody(endStream)
	client := newTestClient(t, staticResponseTransport(body), Config{})

	// When
	_, err := client.Run(context.Background(), validRunInput(), func(cursorproto.ServerEvent) error { return nil })

	// Then
	require.ErrorIs(t, err, ErrInvalidArgument)
	require.ErrorContains(t, err, "Cursor rejected the request")
	require.NotContains(t, err.Error(), "checkpoint rejected")
}

func Test_Client_Run_rejects_clean_connect_end_stream_before_turn_end(t *testing.T) {
	// Given
	endStream := connectFrame([]byte(`{}`))
	endStream[0] = 0x02
	body := newChunkBody(connectFrame(textServerMessage("partial")), endStream)
	client := newTestClient(t, staticResponseTransport(body), Config{})

	// When
	_, err := client.Run(context.Background(), validRunInput(), func(cursorproto.ServerEvent) error { return nil })

	// Then
	require.EqualError(t, err, "Cursor stream ended before turn end")
}

func Test_Client_Run_redacts_connect_end_stream_error_details(t *testing.T) {
	// Given
	endStream := connectFrame([]byte(`{"error":{"code":"permission_denied","message":"account@example.com bearer-secret"}}`))
	endStream[0] = 0x02
	body := newChunkBody(endStream)
	client := newTestClient(t, staticResponseTransport(body), Config{})

	// When
	_, err := client.Run(context.Background(), validRunInput(), func(cursorproto.ServerEvent) error { return nil })

	// Then
	require.EqualError(t, err, "Cursor stream failed: permission_denied")
	require.NotContains(t, err.Error(), "account@example.com")
	require.NotContains(t, err.Error(), "bearer-secret")
}

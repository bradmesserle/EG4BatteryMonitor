package endpoints

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"time"

	"github.com/eg4/battery/monitor/internal"
	"github.com/eg4/battery/monitor/internal/eg4"
	"github.com/labstack/echo/v5"
	"github.com/starfederation/datastar-go/datastar"
)

type Signals struct {
	Content   string `json:"content"`
	Streaming bool   `json:"streaming"`
}

// BatteryStatusStreamHandler handles real-time streaming of the device's battery status to the client via the HTTP connection.
func BatteryStatusStreamHandler(c *echo.Context) error {

	var in Signals
	_ = datastar.ReadSignals(c.Request(), &in)

	ctx, cancel := context.WithTimeout(c.Request().Context(), 45*time.Second)
	defer cancel()

	// NewSSE sets the SSE headers and returns a generator bound to this request.
	sse := datastar.NewSSE(c.Response(), c.Request())

	// Flip the `streaming` signal on so front end can process the streaming data.
	// This is a datastar-patch-signals SSE event.
	_ = sse.MarshalAndPatchSignals(map[string]any{"streaming": true})

	//Subscribe to the topic and post on the stream
	eventBusError := internal.EventBus.Subscribe("batteryStatus", func(row eg4.Row) {

		//Check to see if the SSE connection is still open
		if !sse.IsClosed() {

			out := struct {
				Timestamp string `json:"timestamp"`
				eg4.Row
			}{Timestamp: time.Now().Format("2006-01-02T15:04:05"), Row: row}
			byteString, _ := json.Marshal(out)

			err := sse.ExecuteScript(fmt.Sprintf("updateFields(%s)", string(byteString)))
			if err != nil {
				log.Println(err)
			}

		}

	})

	if eventBusError != nil {
		log.Println(eventBusError)
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				slog.Info("SSE Timeout to client")
				return nil
			}

		case <-c.Request().Context().Done():
			return sse.MarshalAndPatchSignals(map[string]any{"streaming": false})
		}
	}

}

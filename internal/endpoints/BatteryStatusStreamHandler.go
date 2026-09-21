package endpoints

import (
	"fmt"
	"log"
	"strconv"

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

	// NewSSE sets the SSE headers and returns a generator bound to this request.
	sse := datastar.NewSSE(c.Response(), c.Request())

	// Flip the `streaming` signal on so front end can process the streaming data.
	// This is a datastar-patch-signals SSE event.
	_ = sse.MarshalAndPatchSignals(map[string]any{"streaming": true})

	//Subscribe to the topic and post on the stream
	_ = internal.EventBus.Subscribe("batteryStatus", func(row eg4.Row) {

		//Check to see if the SSE connection is still open
		if !sse.IsClosed() {

			socString := strconv.Itoa(*row.SOC)
			err := sse.ExecuteScript(fmt.Sprintf(`update("%s")`, socString))
			if err != nil {
				log.Println(err)
			}
		}

	})

	for {
		select {
		case <-c.Request().Context().Done():
			return sse.MarshalAndPatchSignals(map[string]any{"streaming": false})
		}
	}

}

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/eg4/battery/monitor/internal"
	"github.com/eg4/battery/monitor/internal/eg4"
	"github.com/eg4/battery/monitor/internal/endpoints"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {

	channel := flag.String("channel", "/dev/ttyUSB0", "adapter serial device")
	bitrate := flag.Int("bitrate", 500000, "CAN bitrate (EG4 = 500000)")
	serialBaud := flag.Int("serial-baud", 2000000, "adapter USB serial baud")
	mode := flag.String("mode", "silent", "silent = listen-only (safe); normal = also ACK")
	intervalSec := flag.Float64("interval", 5.0, "seconds between snapshots (ignored for raw)")
	flag.Parse()

	//Connect to EG4 Battery VIA Serial Port
	config := eg4.SerialConfig{
		Channel:     *channel,
		Bitrate:     *bitrate,
		SerialBaud:  *serialBaud,
		Mode:        *mode,
		IntervalSec: *intervalSec,
	}

	err := eg4.GetBatteryInfo(config)
	if err != nil {
		//return
	}

	//Setup web server
	setupWebServer()

}

// setupWebServer Setup web server
func setupWebServer() {

	//Echo web server
	app := echo.New()

	//Static Files
	app.StaticFS("/", echo.MustSubFS(internal.StaticFiles, ""))

	//logger
	app.Use(middleware.RequestLogger())

	//app.Use(middleware.Recover())
	//app.Use(middleware.CORS())

	app.GET("/", func(c *echo.Context) error { return endpoints.Home(c) })

	//Console output SSE
	app.GET("/batteryStatusStream", func(c *echo.Context) error { return endpoints.BatteryStatusStreamHandler(c) })

	// Start the server
	sc := echo.StartConfig{
		Address: ":8080",
		BeforeServeFunc: func(s *http.Server) error {
			s.WriteTimeout = 0 // IMPORTANT: disable for SSE
			return nil
		},
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM) // start shutdown process on ctrl+c
	defer cancel()

	// Start the server
	if err := sc.Start(ctx, app); err != nil {
		app.Logger.Error("failed to start server", "error", err)
	}

}

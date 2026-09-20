package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/eg4/battery/monitor/internal"
	"github.com/eg4/battery/monitor/internal/endpoints"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func main() {

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
	//app.GET("/consoleStream", func(c *echo.Context) error { return endpoints.ConsoleLogStreamHandler(c) })

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

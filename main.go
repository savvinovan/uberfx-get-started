package main

import (
	"context"
	"fmt"
	"github.com/samber/slog-zap/v2"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
)

func main() {
	fx.New(
		fx.Provide(NewLogger),
		fx.WithLogger(func(log *slog.Logger) fxevent.Logger {
			return &fxevent.SlogLogger{Logger: log}
		}),
		fx.Supply(&Config{Env: "development"}),
		fx.Provide(
			NewHTTPServer,
			AsRoute(NewEchoHandler),
			AsRoute(NewHelloHandler),
			fx.Annotate(
				NewServeMux,
				fx.ParamTags("", `group:"routes"`),
			),
		),
		fx.Invoke(func(server *http.Server) {}),
		fx.Decorate(func(l *slog.Logger, c *Config) *slog.Logger {
			return l.With(slog.String("app", c.Env))
		}),
	).Run()
}

type Config struct {
	Env string `json:"env"`
}

// NewHTTPServer builds an HTTP server that will begin serving requests
// when the Fx application starts.
func NewHTTPServer(lc fx.Lifecycle, cfg *Config, mux *http.ServeMux) *http.Server {
	srv := &http.Server{Addr: ":8098", Handler: mux}
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}
			fmt.Println("Starting HTTP server at", srv.Addr, "in", cfg.Env, "mode")
			go func() {
				err := srv.Serve(ln)
				if err != nil {
					fmt.Println("HTTP server error:", err)
				}
			}()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
	return srv
}

// EchoHandler is an http.Handler that copies its request body
// back to the response.
type EchoHandler struct {
	log *slog.Logger
}

// NewEchoHandler builds a new EchoHandler.
func NewEchoHandler(l *slog.Logger) *EchoHandler {
	return &EchoHandler{
		log: l,
	}
}

// ServeHTTP handles an HTTP request to the /echo endpoint.
func (h *EchoHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.log.Info("Handling request", slog.String("path", r.URL.Path))
	if _, err := io.Copy(w, r.Body); err != nil {
		_, err := fmt.Fprintln(os.Stderr, "Failed to handle request:", err)
		if err != nil {
			fmt.Println("Failed to write error:", err)
		}

	}
}

func (h *EchoHandler) Pattern() string {
	return "/echo"
}

// NewServeMux builds a ServeMux that will route requests
// to the given EchoHandler.
func NewServeMux(lc fx.Lifecycle, routes []Route) *http.ServeMux {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			fmt.Println("starting mux")
			return nil
		},
		OnStop: func(ctx context.Context) error {
			fmt.Println("stopping mux")
			return nil
		},
	})
	mux := http.NewServeMux()
	for _, route := range routes {
		mux.Handle(route.Pattern(), route)
	}
	return mux
}

func NewLogger() *slog.Logger {
	z, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
	return slog.New(slogzap.Option{Logger: z}.NewZapHandler())
}

type Route interface {
	http.Handler
	Pattern() string
}

// HelloHandler is an HTTP handler that
// prints a greeting to the user.
type HelloHandler struct {
	log *slog.Logger
}

// NewHelloHandler builds a new HelloHandler.
func NewHelloHandler(log *slog.Logger) *HelloHandler {
	return &HelloHandler{log: log}
}

func (*HelloHandler) Pattern() string {
	return "/hello"
}

func (h *HelloHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("Failed to read request", slog.String("err", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := fmt.Fprintf(w, "Hello, %s\n", body); err != nil {
		h.log.Error("Failed to write response", slog.String("err", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}

// AsRoute annotates the given constructor to state that
// it provides a route to the "routes" group.
func AsRoute(f any) any {
	return fx.Annotate(
		f,
		fx.As(new(Route)),
		fx.ResultTags(`group:"routes"`),
	)
}

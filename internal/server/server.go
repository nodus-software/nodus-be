package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"nodus-health/internal/middleware"
	"nodus-health/pkg/logger"
)

// RouteRegistrar is implemented by each domain's handler to attach its own
// routes to the router.
type RouteRegistrar interface {
	RegisterRoutes(r chi.Router)
}

type Config struct {
	Port           string
	AllowedOrigins []string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	IdleTimeout    time.Duration
	ShutdownGrace  time.Duration
	RequestTimeout time.Duration
	TenantResolver middleware.TenantResolver
	TenantPool     *pgxpool.Pool
}

type Server struct {
	httpServer *http.Server
	log        *logger.Logger
	cfg        Config
}

// New assembles the router (cross-cutting middleware + each domain's
// routes) and wraps it in an http.Server. Each domain's handler builds its
// own auth-specific middleware (e.g. middleware.Authenticate) internally
// when registering its routes.
func New(cfg Config, log *logger.Logger, registrars ...RouteRegistrar) *Server {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(log))
	r.Use(middleware.Logging(log))
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.CORS(cfg.AllowedOrigins))
	r.Use(middleware.Timeout(cfg.RequestTimeout))
	if cfg.TenantResolver != nil {
		r.Use(middleware.ResolveTenant(cfg.TenantResolver))
	}
	if cfg.TenantPool != nil {
		r.Use(middleware.TenantTransaction(cfg.TenantPool))
	}

	for _, reg := range registrars {
		reg.RegisterRoutes(r)
	}

	return &Server{
		log: log,
		cfg: cfg,
		httpServer: &http.Server{
			Addr:         ":" + cfg.Port,
			Handler:      r,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
			IdleTimeout:  cfg.IdleTimeout,
		},
	}
}

// Run starts the HTTP server and blocks until ctx is cancelled, at which
// point it drains in-flight requests (bounded by ShutdownGrace) before
// returning.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.log.Info("server starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	s.log.Info("server shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.cfg.ShutdownGrace)
	defer cancel()

	if err := s.httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	s.log.Info("server stopped")
	return nil
}

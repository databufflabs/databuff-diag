package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/databufflabs/databuff-diag/internal/agent"
	"github.com/databufflabs/databuff-diag/internal/skill"
	"github.com/databufflabs/databuff-diag/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type appStores struct {
	Config      *store.ConfigStore
	Sessions    *store.SessionStore
	Attachments *store.AttachmentStore
}

func initStores() (*appStores, error) {
	cfgStore, err := store.NewConfigStore()
	if err != nil {
		return nil, fmt.Errorf("init config store: %w", err)
	}
	sessionStore, err := store.NewSessionStore()
	if err != nil {
		return nil, fmt.Errorf("init session store: %w", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("resolve working directory: %w", err)
	}
	sessionStore.SetWorkspaceRoot(wd)
	attachmentStore, err := store.NewAttachmentStore()
	if err != nil {
		return nil, fmt.Errorf("init attachment store: %w", err)
	}
	return &appStores{
		Config:      cfgStore,
		Sessions:    sessionStore,
		Attachments: attachmentStore,
	}, nil
}

// New returns an HTTP handler for the databuff-diag API.
func New() (http.Handler, error) {
	stores, err := initStores()
	if err != nil {
		return nil, err
	}
	return NewWithStores(stores.Config, stores.Sessions, stores.Attachments), nil
}

// NewWithStore returns an HTTP handler using the given config store (for tests).
func NewWithStore(cfgStore *store.ConfigStore) http.Handler {
	sessionStore := store.NewSessionStoreAt(sessionDirForConfig(cfgStore))
	attachmentStore := store.NewAttachmentStoreAt(filepath.Join(filepath.Dir(cfgStore.Path()), "uploads"))
	return NewWithStores(cfgStore, sessionStore, attachmentStore)
}

// NewWithStores returns an HTTP handler with explicit stores (for tests).
func NewWithStores(cfgStore *store.ConfigStore, sessionStore *store.SessionStore, attachmentStore *store.AttachmentStore) http.Handler {
	skillLoader := newSkillLoader(cfgStore)
	ag := agent.New(sessionStore)
	ag.Skills = skillLoader
	ag.Attachments = attachmentStore
	ag.ConfigStore = cfgStore

	auth := NewAuthManager(cfgStore)
	authHandler := &AuthHandler{Auth: auth}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", HealthHandler)

	r.Post("/api/auth/login", authHandler.Login)
	r.Route("/api/auth", func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/me", authHandler.Me)
		r.Post("/logout", authHandler.Logout)
	})

	sessions := &SessionsHandler{
		ConfigStore:  cfgStore,
		SessionStore: sessionStore,
		Agent:        ag,
	}
	chat := &ChatHandler{
		ConfigStore:     cfgStore,
		SessionStore:    sessionStore,
		AttachmentStore: attachmentStore,
		Agent:           ag,
	}
	reportExport := &ReportExportHandler{SessionStore: sessionStore}
	envBundle := &EnvBundleHandler{ConfigStore: cfgStore}
	envBundleDownload := &EnvBundleDownloadHandler{
		ReportsDir: filepath.Join(filepath.Dir(cfgStore.Path()), "reports"),
	}
	workspace := &WorkspaceHandler{SessionStore: sessionStore}

	r.Route("/api", func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/providers", (&ProvidersHandler{}).ServeHTTP)
		r.Method("GET", "/config", &ConfigHandler{Store: cfgStore})
		r.Method("PUT", "/config", &ConfigHandler{Store: cfgStore})
		r.Method("POST", "/llm/test", &LLMTestHandler{Store: cfgStore})
		r.Method("POST", "/exec/local", &ExecLocalHandler{})
		r.Method("POST", "/exec/ssh", &ExecSSHHandler{ConfigStore: cfgStore})
		r.Method("GET", "/report/export", reportExport)
		r.Method("POST", "/report/export", reportExport)
		r.Method("POST", "/collect/env-bundle", envBundle)
		r.Method("GET", "/collect/env-bundle/{filename}", envBundleDownload)
		r.Method("POST", "/chat", chat)
		r.Method("POST", "/upload", &UploadHandler{Attachments: attachmentStore})
		r.Method("GET", "/attachments/{id}", &AttachmentHandler{Attachments: attachmentStore})
		r.Get("/skills", (&SkillsHandler{Loader: skillLoader}).ServeHTTP)

		r.Get("/workspace", workspace.Info)
		r.Get("/workspace/tree", workspace.Tree)
		r.Get("/workspace/file", workspace.File)
		r.Post("/workspace/file", workspace.CreateFile)
		r.Put("/workspace/file", workspace.UpdateFile)
		r.Delete("/workspace/file", workspace.DeleteFile)
		r.Post("/workspace/upload", workspace.UploadFiles)
		r.Post("/workspace/lint", workspace.LintFile)

		r.Get("/sessions", sessions.List)
		r.Post("/sessions", sessions.Create)
		r.Get("/sessions/{id}", sessions.Get)
		r.Delete("/sessions/{id}", sessions.Delete)
		r.Patch("/sessions/{id}", sessions.Patch)
		r.Post("/sessions/{id}/message", sessions.Message)
		r.Post("/sessions/{id}/approve", sessions.Approve)
	})

	r.Handle("/*", authenticatedStaticHandler(auth))

	return r
}

func sessionDirForConfig(cfgStore *store.ConfigStore) string {
	return filepath.Join(filepath.Dir(cfgStore.Path()), "sessions")
}

func newSkillLoader(cfgStore *store.ConfigStore) *skill.Loader {
	cfg, err := cfgStore.Load()
	if err != nil {
		loader := skill.NewLoader(nil)
		_ = loader.Load()
		return loader
	}
	loader := skill.NewLoader(cfg.Skills.Dirs)
	_ = loader.Load()
	return loader
}

// ListenAndServe starts the HTTP server on addr (e.g. ":8787").
func ListenAndServe(addr string) error {
	stores, err := initStores()
	if err != nil {
		return err
	}
	NewSessionCleanup(stores.Config, stores.Sessions).Start(context.Background())

	handler := NewWithStores(stores.Config, stores.Sessions, stores.Attachments)
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen and serve: %w", err)
	}
	return nil
}

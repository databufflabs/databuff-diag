package server

import (
	"fmt"

	"github.com/databufflabs/databuff-diag/internal/store"
)

// PrintStartupBanner writes the success message and login credentials to stdout.
func PrintStartupBanner(addr string) {
	fmt.Printf("✓ databuff-diag 启动成功，访问 %s\n", ServeURL(addr))
	printAuthCredentials()
}

func printAuthCredentials() {
	cfgStore, err := store.NewConfigStore()
	if err != nil {
		return
	}
	cfg, err := cfgStore.Load()
	if err != nil {
		return
	}
	fmt.Printf("  用户名: %s\n", cfg.Auth.Username)
	fmt.Printf("  密码:   %s\n", cfg.Auth.Password)
}

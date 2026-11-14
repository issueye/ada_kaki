// The AI operator's app-side state: the connection probe behind the
// settings page's "测试连接" button. The dock, the agent loop and the
// title-bar button join this file when their stages land.

package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"adakaki/internal/ai"
)

// probeAI asks the configured provider for a one-token answer, off the UI
// thread, and reports the outcome beside the button.
func (a *app) probeAI() {
	cfg := a.store.Settings.AI
	if a.aiProbe == "测试中…" {
		return
	}
	a.aiProbe = "测试中…"
	a.aiProbeOK = false
	a.refreshSettingsWindow()

	providerName := aiLabel(cfg.Provider, aiProviderOptions)
	provider := ai.Provider{
		Name: providerName, BaseURL: strings.TrimSpace(cfg.BaseURL),
		Model: strings.TrimSpace(cfg.Model), KeyEnv: strings.TrimSpace(cfg.APIKeyEnv),
	}

	go func() {
		key, err := ai.KeyFromEnv(provider.KeyEnv)
		if err != nil {
			a.finishProbe(false, err.Error())
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		client := ai.NewClient(provider, key, nil)
		start := time.Now()
		resp, err := client.Chat(ctx, ai.Request{
			Messages:  []ai.Message{{Role: "user", Content: "ping"}},
			MaxTokens: 1,
		}, nil)
		if err != nil {
			a.finishProbe(false, err.Error())
			return
		}
		elapsed := time.Since(start).Round(time.Millisecond)
		msg := "连接成功 · " + provider.Model + " · " + elapsed.String()
		if resp.Usage.Total() > 0 {
			msg += fmt.Sprintf(" · %d tokens", resp.Usage.Total())
		}
		a.finishProbe(true, msg)
	}()
}

// finishProbe publishes the outcome and repaints the settings window.
func (a *app) finishProbe(ok bool, msg string) {
	a.aiProbe = msg
	a.aiProbeOK = ok
	a.refreshSettingsWindow()
}

// refreshSettingsWindow redraws the settings window from any goroutine.
func (a *app) refreshSettingsWindow() {
	w := a.settingsWin
	if w == nil {
		return
	}
	w.Update(func() {})
}

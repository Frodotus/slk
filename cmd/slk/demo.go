package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/gammons/slk/internal/avatar"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/demo"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// demoApp bundles the seeded *ui.App with the bits needed to warm avatars
// after the program starts (so their kitty uploads happen in the render
// loop, like the real app — uploading mid-first-frame loses them and the
// placeholders composite stale images from the terminal's graphics cache).
type demoApp struct {
	app         *ui.App
	avatarCache *avatar.Cache
	avatars     map[string]image.Image // userID -> generated avatar
	people      []demo.Person
}

// loadDemoConfig loads the user's config for theme/appearance, falling
// back to defaults so --demo works even without a config file.
func loadDemoConfig() config.Config {
	cfg, err := config.Load(filepath.Join(xdgConfig(), "config.toml"))
	if err != nil {
		cfg = config.Default()
	}
	return cfg
}

// seedDemoApp builds the seeded demo app — no Slack connection, DB, or
// network. The inline photo is injected through the fetcher cache; avatars
// are generated here but rendered later (see warmAvatars).
func seedDemoApp(cfg config.Config) *demoApp {
	styles.LoadCustomThemes(filepath.Join(xdgConfig(), "themes"))
	styles.Apply(cfg.Appearance.Theme, cfg.Theme)

	app := ui.NewApp()
	app.SetPanelBorders(cfg.Appearance.PanelBorders)

	scene := demo.Build()

	// Detect the protocol (kitty for crisp pixels) and run the same
	// interactive kitty probe run() does before bubbletea takes the
	// terminal. Emoji image-mode stays OFF (plain unicode; the scene uses
	// only emoji that render correctly without the image pipeline).
	proto := imgpkg.Detect(imgpkg.CaptureEnv(), cfg.Appearance.ImageProtocol)
	if proto == imgpkg.ProtoKitty && term.IsTerminal(int(os.Stdin.Fd())) {
		if state, err := term.MakeRaw(int(os.Stdin.Fd())); err == nil {
			ok := imgpkg.ProbeKittyGraphics(os.Stdout, os.Stdin, 200*time.Millisecond)
			_ = term.Restore(int(os.Stdin.Fd()), state)
			if !ok {
				proto = imgpkg.ProtoHalfBlock
			}
		}
	}

	// Inline photo: pre-load the fetcher so the message pane renders it
	// from cache (its flush path uploads reliably).
	fetcher := imgpkg.NewFetcher(nil, nil)
	fetcher.SetLocalImage(demo.DemoImageFileID, demo.InlineImage())
	pxW, pxH := imgpkg.CellPixels(int(os.Stdout.Fd()))
	app.SetImageProtocol(proto)
	app.SetImageContext(imgrender.ImageContext{
		Protocol:    proto,
		Fetcher:     fetcher,
		KittyRender: imgpkg.KittyRendererInstance(),
		CellPixels:  image.Pt(pxW, pxH),
		MaxRows:     cfg.Appearance.MaxImageRows,
		MaxCols:     cfg.Appearance.MaxImageCols,
	})

	// Avatars: generate the images now; render them in warmAvatars after
	// the program loop is up. The AvatarFunc only reads the cache.
	avatarCache := avatar.NewCache(fetcher, imgpkg.KittyRendererInstance(), proto == imgpkg.ProtoKitty)
	avatars := make(map[string]image.Image, len(scene.People))
	for _, p := range scene.People {
		avatars[p.ID] = demo.GenerateAvatar(p.Initials, p.Color, 96)
	}
	app.SetAvatarFunc(func(id string) string { return avatarCache.Get(id) })

	app.SetCurrentUserID(scene.CurrentUserID)
	app.SetUserNames(scene.UserNames)
	app.SetWorkspaces(scene.Workspaces)
	app.SetSectionsProvider(scene.SectionsProvider())
	app.SetChannels(scene.Channels)
	app.SetInitialChannel(scene.ActiveChannelID, scene.ActiveChannelName, scene.Messages)
	app.ShowThread(scene.Thread.Parent, scene.Thread.Replies, scene.Thread.ChannelID, scene.Thread.ThreadTS)

	return &demoApp{app: app, avatarCache: avatarCache, avatars: avatars, people: scene.People}
}

// warmAvatars renders each generated avatar (firing its kitty upload) and
// re-renders the affected rows — run from a goroutine after the program
// starts so uploads land while the alt-screen is up. Mirrors the real
// app's lazy avatar-preload + AvatarReadyMsg flow.
func (d *demoApp) warmAvatars(p *tea.Program) {
	time.Sleep(120 * time.Millisecond) // let bubbletea enter the alt-screen
	for _, person := range d.people {
		img, ok := d.avatars[person.ID]
		if !ok {
			continue
		}
		d.avatarCache.SetLocalAvatar(person.ID, img)
		p.Send(messages.AvatarReadyMsg{UserID: person.ID})
	}
}

// runDemo launches the real TUI seeded with a curated fake scene, for
// taking screenshots. No tokens, DB, WebSocket, or network.
func runDemo() error {
	d := seedDemoApp(loadDemoConfig())
	p := tea.NewProgram(d.app)
	go d.warmAvatars(p)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("demo: %w", err)
	}
	return nil
}

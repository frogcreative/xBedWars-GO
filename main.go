// Command bedwars is the entry point of the NewBedwars game mode for a Dragonfly server.
//
// It is a port of the PocketMine-MP 5 "NewBedwars" plugin: arenas, lobby, teams, shops, upgrades,
// traps and mobs behave like the original, driven by the same config.yml and arena files.
package main

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/df-mc/dragonfly/server"
	"github.com/df-mc/dragonfly/server/cmd"
	"github.com/df-mc/dragonfly/server/player/chat"

	"github.com/frogcreative/bedwars/assets"
	"github.com/frogcreative/bedwars/internal/cfg"
	"github.com/frogcreative/bedwars/internal/cmds"
	"github.com/frogcreative/bedwars/internal/events"
	"github.com/frogcreative/bedwars/internal/game"
	"github.com/frogcreative/bedwars/internal/worlds"
)

// dataDir is the folder holding config.yml, ops.txt, arenas, map snapshots and skins.
const dataDir = "plugin_data/NewBedwars"

// main wires the config, world manager, game plugin, command and player handler, then serves
// players until shutdown.
func main() {
	log := slog.Default()
	chat.Global.Subscribe(chat.StdoutSubscriber{})

	if err := cfg.Load(dataDir); err != nil {
		log.Error("failed to load config.yml", "err", err)
		os.Exit(1)
	}
	initDataFolder(log)
	loadOps()

	uc := server.DefaultConfig()
	conf, err := uc.Config(log)
	if err != nil {
		log.Error("failed to read the server configuration", "err", err)
		os.Exit(1)
	}
	srv := conf.New()
	srv.CloseOnProgramEnd()

	wm := worlds.New("worlds", filepath.Join(dataDir, "Cache"), log, conf.Entities)
	game.P = game.NewPlugin(dataDir, log, wm, srv.World)
	if err := game.P.LoadArenas(); err != nil {
		log.Error("failed to load arenas", "err", err)
		os.Exit(1)
	}

	cmd.Register(cmd.New("newbedwars", "NewBedWars Commands", []string{"nbw", "bedwars", "bw"}, cmds.Root{}, cmds.Sub{}))

	srv.Listen()
	for p := range srv.Accept() {
		events.Attach(p)
	}
	game.P.SaveArenas()
}

// initDataFolder creates the data sub-folders and extracts the bundled generator skins on first
// run.
func initDataFolder(log *slog.Logger) {
	for _, d := range []string{"Arenas", "Cache", "Skin"} {
		_ = os.MkdirAll(filepath.Join(dataDir, d), 0o755)
	}
	// Seed only missing skins so local replacements survive restarts.
	for _, n := range []string{"diamond.png", "emerald.png", "invisible.png"} {
		dst := filepath.Join(dataDir, "Skin", n)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		b, err := assets.Skin.ReadFile("skin/" + n)
		if err != nil {
			log.Error("bundled skin is missing", "file", n)
			continue
		}
		_ = os.WriteFile(dst, b, 0o644)
	}
}

// loadOps reads ops.txt (one player name per line, "#" starts a comment) and registers the
// operators who may use /bw and operator chat commands.
func loadOps() {
	path := filepath.Join(dataDir, "ops.txt")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, []byte("# One player name per line: these players may use /bw.\n"), 0o644)
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		game.Ops[strings.ToLower(l)] = true
	}
}

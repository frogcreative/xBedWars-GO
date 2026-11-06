// Package game implements the BedWars match: arenas, lobby, teams, generators, shops, upgrades,
// traps, mobs and the match lifecycle.
//
// All mutable game state is guarded by a single mutex (Plugin.Mu). Event handlers and commands
// take it briefly and never perform blocking world operations while holding it; world work is
// queued through package plat or scheduled with World.Exec.
package game

import (
	"log/slog"
	"sync"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/arena"
	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
	"github.com/frogcreative/bedwars/internal/worlds"
)

// Prefix is prepended to every plugin message.
const Prefix = "§o§l§bNew§cBed§fWars§r§7 > "

// Plugin holds the shared state of the game mode.
type Plugin struct {
	DataDir      string
	Log          *slog.Logger
	Worlds       *worlds.Manager
	DefaultWorld func() *world.World

	// Mu guards every field below and all game state; hold it only for short, non-blocking sections.
	Mu      sync.Mutex
	// Arenas are the loaded arenas by world name.
	Arenas  map[string]*Game
	// Setters maps an operator name to the arena they are setting up.
	Setters map[string]*Game
	// Online maps a player name to their wrapper.
	Online  map[string]*plat.Player
	// InGame maps a player name to the arena they are in.
	InGame  map[string]*Game
	// Pos is the last known position of each player, updated on movement.
	Pos     map[string]util.Vec
}

// P is the process-wide plugin instance.
var P *Plugin

// NewPlugin creates the plugin state.
func NewPlugin(dataDir string, log *slog.Logger, w *worlds.Manager, def func() *world.World) *Plugin {
	return &Plugin{
		DataDir: dataDir, Log: log, Worlds: w, DefaultWorld: def,
		Arenas:  map[string]*Game{},
		Setters: map[string]*Game{},
		Online:  map[string]*plat.Player{},
		InGame:  map[string]*Game{},
		Pos:     map[string]util.Vec{},
	}
}

// LoadArenas loads every arena file and creates its game.
func (p *Plugin) LoadArenas() error {
	all, err := arena.LoadAll(p.DataDir)
	if err != nil {
		return err
	}
	p.Mu.Lock()
	defer p.Mu.Unlock()
	for name, d := range all {
		p.Arenas[name] = newGame(p, name, d, false)
	}
	return nil
}

// SaveArenas stops the games, restores the original maps and persists every arena. Call it on
// shutdown.
func (p *Plugin) SaveArenas() {
	p.Mu.Lock()
	defer p.Mu.Unlock()
	for name, g := range p.Arenas {
		g.stopLoop()
		if g.World != nil {
			spawn := p.DefaultSpawn()
			for _, pl := range g.Players {
				pl.TeleportTo(p.DefaultWorld(), spawn)
			}
			p.Worlds.Unload(arena.Str(g.Data.World))
			if err := p.Worlds.RestoreMap(arena.Str(g.Data.World)); err != nil {
				p.Log.Warn("failed to restore the map on shutdown", "arena", name, "err", err)
			}
		}
		if err := arena.Save(p.DataDir, name, g.Data); err != nil {
			p.Log.Error("failed to save the arena", "arena", name, "err", err)
		}
	}
}

// DefaultSpawn returns the spawn point of the server default world.
func (p *Plugin) DefaultSpawn() mgl64.Vec3 {
	sp := p.DefaultWorld().Spawn()
	return mgl64.Vec3{float64(sp[0]) + 0.5, float64(sp[1]), float64(sp[2]) + 0.5}
}

// PlayerOnline registers a joined player.
func (p *Plugin) PlayerOnline(pl *plat.Player) {
	p.Mu.Lock()
	p.Online[pl.Name] = pl
	p.Mu.Unlock()
}

// PlayerOffline removes a player who left, taking them out of any arena or setup session.
func (p *Plugin) PlayerOffline(name string) {
	p.Mu.Lock()
	pl := p.Online[name]
	if pl != nil {
		pl.Close()
	}
	if g, ok := p.InGame[name]; ok && pl != nil {
		g.disconnectPlayer(pl, "", false)
	}
	delete(p.Setters, name)
	delete(p.Online, name)
	delete(p.InGame, name)
	delete(p.Pos, name)
	p.Mu.Unlock()
}

package game

import (
	"fmt"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/arena"
	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// Match phases.
const (
	PhaseLobby   = 0
	PhaseGame    = 1
	PhaseRestart = 3
)

// Game is one arena and the match running in it.
type Game struct {
	P    *Plugin
	Name string
	Data *arena.Data

	// Setup is true while the arena definition is incomplete or being edited.
	Setup bool
	World *world.World
	Phase int

	// Players are the active participants; eliminated players move to Spectators.
	Players    []*plat.Player
	Spectators map[string]*plat.Player
	// Teams maps a team name to its living members.
	Teams      map[string]map[string]*plat.Player
	// TempTeam remembers the team of spectators after elimination.
	TempTeam   map[string]string

	// UtilityArena stores the upgrade levels per team (generator, sharpness, protection, haste,
	// health).
	UtilityArena map[string]map[string]int
	// RespawnC is the respawn countdown in seconds per player.
	RespawnC     map[string]int
	// IfMilk is the remaining seconds of trap immunity per player.
	IfMilk       map[string]int
	Kills        map[string]int
	FinalKills   map[string]int
	BedsBroken   map[string]int

	// Gens are the active resource generators.
	Gens        []*Gen
	// PlacedBlock tracks blocks placed by players; only these can be broken or blown up.
	PlacedBlock map[cube.Pos]bool
	ArmorType   map[string]string
	// LastDamager is the last player to hurt each player, used for kill credit.
	LastDamager map[string]string
	PickaxeType map[string]int
	AxeType     map[string]int
	IfShears    map[string]bool
	Mobs        map[*world.EntityHandle]*Mob
	// Traps is the queue of purchased traps per team (at most three).
	Traps       map[string][]string
	// CauseOverride forces the death cause of the next damage a player takes (explosions count as
	// projectiles).
	CauseOverride map[string]string

	T Timers

	// beds caches whether each team bed is standing.
	beds map[string]bool
	stop chan struct{}
}

// newGame creates the game for an arena. An arena whose definition is incomplete starts in setup
// mode.
func newGame(p *Plugin, name string, d *arena.Data, fresh bool) *Game {
	g := &Game{
		P: p, Name: name, Data: d,
		Spectators: map[string]*plat.Player{}, Teams: map[string]map[string]*plat.Player{},
		TempTeam: map[string]string{}, UtilityArena: map[string]map[string]int{},
		RespawnC: map[string]int{}, IfMilk: map[string]int{},
		Kills: map[string]int{}, FinalKills: map[string]int{}, BedsBroken: map[string]int{},
		beds: map[string]bool{}, stop: make(chan struct{}),
		PlacedBlock: map[cube.Pos]bool{}, ArmorType: map[string]string{}, LastDamager: map[string]string{},
		PickaxeType: map[string]int{}, AxeType: map[string]int{}, IfShears: map[string]bool{}, Traps: map[string][]string{}, Mobs: map[*world.EntityHandle]*Mob{}, CauseOverride: map[string]string{},
	}
	g.Setup = !g.enable(false)
	g.T.reload(g)
	go g.run()
	if !g.Setup {
		g.loadArena(false)
	}
	return g
}

// NewSetupGame creates an empty arena used while an operator is setting it up.
func NewSetupGame(p *Plugin, name string) *Game { return newGame(p, name, arena.New(), true) }

// stopLoop stops the per-second ticker.
func (g *Game) stopLoop() {
	select {
	case <-g.stop:
	default:
		close(g.stop)
	}
}

// Delete stops the arena ticker when the arena is deleted.
func (g *Game) Delete() { g.stopLoop() }

// enable validates the arena and marks it enabled, optionally loading it.
func (g *Game) enable(load bool) bool {
	if err := g.Data.Validate(); err != nil {
		g.P.Log.Error(err.Error())
		return false
	}
	name := arena.Str(g.Data.World)
	if !g.P.Worlds.IsGenerated(name) {
		g.P.Log.Error("Cannot find world with name " + name + " in server worlds")
		return false
	}
	g.Data.Enabled = true
	g.Setup = false
	if load {
		g.loadArena(false)
	}
	return true
}

// Enable is enable for callers that already hold the plugin lock.
func (g *Game) Enable(load bool) bool { return g.enable(load) }

// LoadArena loads (or reloads) the arena world and resets the match state.
func (g *Game) LoadArena(restart bool) { g.loadArena(restart) }

// loadArena prepares the arena for a new round: timers, world (restored from its snapshot on
// restart) and teams.
func (g *Game) loadArena(restart bool) {
	name := arena.Str(g.Data.World)
	g.P.Log.Info("Start load arena " + name)
	if !g.Data.Enabled {
		g.P.Log.Error("Arena is not enabled!")
		return
	}
	g.T.reload(g)
	if restart {
		w, err := g.P.Worlds.LoadMap(name)
		if err != nil {
			g.P.Log.Error(err.Error())
		} else {
			g.World = w
		}
	}
	if g.World == nil {
		w, err := g.P.Worlds.Load(name)
		if err != nil {
			g.P.Log.Error("Arena world not found!", "err", err)
			g.Setup = true
			return
		}
		g.World = w
	}
	g.initTeams()
	g.Phase = PhaseLobby
	g.Players = nil
}

// initTeams clears the team rosters.
func (g *Game) initTeams() {
	if !g.Setup {
		g.Teams = map[string]map[string]*plat.Player{}
		g.UtilityArena = map[string]map[string]int{}
	}
}

// maxPlayers returns the arena capacity.
func (g *Game) maxPlayers() int { return g.Data.Max() * len(g.Data.TeamName) }

// inGame reports whether the player takes part in this arena.
func (g *Game) inGame(pl *plat.Player) bool {
	// In a running match, spectators and players awaiting respawn still belong to the arena.
	if g.Phase == PhaseLobby {
		for _, p := range g.Players {
			if p.Name == pl.Name {
				return true
			}
		}
		return false
	}
	for _, p := range g.Players {
		if p.Name == pl.Name {
			return true
		}
	}
	_, spec := g.Spectators[pl.Name]
	_, resp := g.RespawnC[pl.Name]
	return spec || resp
}

// Join adds a player to the lobby (locking entry point).
func (g *Game) Join(pl *plat.Player) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	g.joinToArena(pl)
}

// joinToArena validates the join request, assigns the first team with room and prepares the
// player.
func (g *Game) joinToArena(pl *plat.Player) {
	if !g.Data.Enabled {
		pl.Msg(Prefix + "§cSorry but you can't join this arena at this moment")
		return
	}
	// A full arena rejects the join request.
	if len(g.Players) >= g.maxPlayers() {
		pl.Msg(Prefix + "§cThis arena full!")
		return
	}
	if g.inGame(pl) {
		return
	}
	if g.Phase == PhaseGame || g.Phase == PhaseRestart {
		pl.Msg(Prefix + "§cArena has been started!")
		return
	}
	if old, ok := g.P.InGame[pl.Name]; ok && old != g {
		old.disconnectPlayer(pl, "", false)
	}

	wp := util.MustVec(arena.Str(g.Data.WaitingSpawn))
	pl.TeleportTo(g.World, mgl64.Vec3{wp.X, wp.Y, wp.Z})
	for _, team := range g.Data.TeamName {
		if g.setTeam(pl, team) {
			break
		}
	}
	g.Players = append(g.Players, pl)
	g.P.InGame[pl.Name] = g

	pl.ClearEffects()
	pl.ClearInventories()
	pl.StopFlying()
	pl.SetAbsorption(0)
	pl.LobbyItems()
	pl.SetGameMode("adventure")
	pl.Restore()
	g.broadcast(util.Msg("messages.join",
		"teamcolor", g.Data.TeamColor[g.getTeam(pl)],
		"player", pl.Name,
		"count", len(g.Players),
		"max", g.maxPlayers()))
}

// ResetWorld drops the reference to the world, for example after it was unloaded.
func (g *Game) ResetWorld() { g.World = nil }

// TeamEntry is one row of the team selector.
type TeamEntry struct {
	Name  string
	Color string
	Count int
}

// TeamEntries returns the rows of the team selector.
func (g *Game) TeamEntries() []TeamEntry {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	out := make([]TeamEntry, 0, len(g.Data.TeamName))
	for _, t := range g.Data.TeamName {
		out = append(out, TeamEntry{Name: t, Color: g.Data.TeamColor[t], Count: g.getCountTeam(t)})
	}
	return out
}

// SelectTeam moves a lobby player to the chosen team when it has room.
func (g *Game) SelectTeam(pl *plat.Player, team string) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	if g.Phase != PhaseLobby || !g.inGame(pl) {
		return
	}
	valid := false
	for _, t := range g.Data.TeamName {
		if t == team {
			valid = true
		}
	}
	if !valid {
		return
	}
	if g.getTeam(pl) == team {
		pl.Msg("You already in this team!")
		return
	}
	if g.getCountTeam(team) >= g.Data.Max() {
		pl.Msg("Team is full")
		return
	}
	g.setTeam(pl, team)
	pl.Msg("You've joined team " + team)
}

// PhaseNow returns the current phase.
func (g *Game) PhaseNow() int {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	return g.Phase
}

// JoinToArena is joinToArena for callers that already hold the plugin lock.
func (g *Game) JoinToArena(pl *plat.Player) { g.joinToArena(pl) }

// Quit removes a player from the arena (locking entry point).
func (g *Game) Quit(pl *plat.Player) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	g.disconnectPlayer(pl, "", false)
}

// removePlayer removes the player from the active roster.
func (g *Game) removePlayer(name string) {
	out := g.Players[:0]
	for _, p := range g.Players {
		if p.Name != name {
			out = append(out, p)
		}
	}
	g.Players = out
}

// disconnectPlayer removes a player from the arena, resets them and sends them to the default
// world. In a running match an emptied team loses its generators and bed.
func (g *Game) disconnectPlayer(pl *plat.Player, _ string, _ bool) {
	switch g.Phase {
	case PhaseLobby:
		g.broadcast(util.Msg("messages.lobby-quit", "player", pl.Name))
		delete(g.Teams[g.getTeam(pl)], pl.Name)
		g.removePlayer(pl.Name)
	default:
		g.removePlayer(pl.Name)
	}
	delete(g.P.InGame, pl.Name)
	pl.RemoveScoreboard()
	pl.Restore()
	pl.ClearInventories()
	pl.SetGameMode("survival")
	pl.ClearEffects()
	pl.SetScoreTag("")
	pl.TeleportTo(g.P.DefaultWorld(), g.P.DefaultSpawn())

	team := g.getTeam(pl)
	if g.Phase == PhaseGame {
		g.broadcast(util.Msg("messages.leave", "teamcolor", g.Data.TeamColor[team], "player", pl.Name))
		count := 0
		for _, mate := range g.Players {
			if g.getTeam(mate) == team {
				count++
			}
		}
		if count <= 0 {
			g.closeTeamGenerators(team)
			g.breakBed(team, nil)
			g.broadcast("")
			g.broadcast(util.Msg("messages.team-eliminated", "teamcolor", g.Data.TeamColor[team], "team", team))
			g.broadcast("")
		}
		g.unsetPlayer(pl)
	}
}

// unsetPlayer drops every per-player match record.
func (g *Game) unsetPlayer(pl *plat.Player) {
	delete(g.Teams[g.getTeam(pl)], pl.Name)
	delete(g.Spectators, pl.Name)
	delete(g.RespawnC, pl.Name)
	delete(g.ArmorType, pl.Name)
	delete(g.IfShears, pl.Name)
	delete(g.AxeType, pl.Name)
	delete(g.PickaxeType, pl.Name)
	delete(g.Kills, pl.Name)
	delete(g.FinalKills, pl.Name)
	delete(g.BedsBroken, pl.Name)
	g.removePlayer(pl.Name)
	pl.SetScoreTag("")
	pl.SetNameTag(pl.Name)
}

// getCountTeam returns the number of players on a team.
func (g *Game) getCountTeam(team string) int { return len(g.Teams[team]) }

// setTeam moves the player to the team when it has room.
func (g *Game) setTeam(pl *plat.Player, team string) bool {
	if g.getCountTeam(team) < g.Data.Max() {
		if cur := g.getTeam(pl); cur != "" {
			delete(g.Teams[cur], pl.Name)
		}
		if g.Teams[team] == nil {
			g.Teams[team] = map[string]*plat.Player{}
		}
		g.Teams[team][pl.Name] = pl
		return true
	}
	return false
}

// getTeam returns the player current team, or the team they belonged to before becoming a
// spectator.
func (g *Game) getTeam(pl *plat.Player) string {
	res := ""
	for _, team := range g.Data.TeamName {
		if t, ok := g.TempTeam[pl.Name]; ok {
			res = t
		}
		if _, ok := g.Teams[team][pl.Name]; ok {
			res = team
		}
	}
	return res
}

// SetTeam is setTeam as a locking entry point.
func (g *Game) SetTeam(pl *plat.Player, team string) bool {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	return g.setTeam(pl, team)
}

// getAliveTeams returns the teams that still have players.
func (g *Game) getAliveTeams() []string {
	var out []string
	for _, t := range g.Data.TeamName {
		if g.getCountTeam(t) >= 1 {
			out = append(out, t)
		}
	}
	return out
}

// statusTeam returns the scoreboard status of a team: bed alive, remaining players or eliminated.
func (g *Game) statusTeam(team string) string {
	if g.beds[team] {
		return "§a" + cfgStr("scoreboard.unicode.team-bed-alive")
	}
	if c := g.getCountTeam(team); c != 0 {
		return fmt.Sprintf("§a%d", c)
	}
	return "§c" + cfgStr("scoreboard.unicode.team-eliminated")
}

// broadcast sends a message to everyone inside the arena.
func (g *Game) broadcast(msg string) {
	for _, p := range g.worldPlayers() {
		p.Msg(msg)
	}
}

// Broadcast is broadcast as a locking entry point.
func (g *Game) Broadcast(msg string) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	g.broadcast(msg)
}

// setColorTag prefixes the name tag with the team color and initial.
func (g *Game) setColorTag(pl *plat.Player) {
	team := g.getTeam(pl)
	if team == "" {
		return
	}
	c := g.Data.TeamColor[team]
	pl.SetNameTag(c + "§l " + util.Ucfirst(team[:1]) + "§r" + c + " " + pl.Name)
}

// teleportToSpawn moves the player to their team spawn.
func (g *Game) teleportToSpawn(pl *plat.Player) {
	v := util.MustVec(g.Data.TeamSpawn[g.getTeam(pl)]).Add(0.5, 0.5, 0.5)
	pl.TeleportTo(g.World, mgl64.Vec3{v.X, v.Y, v.Z})
}

// startGame begins the match: it spawns the NPCs, equips and teleports every player and enters the
// game phase.
func (g *Game) startGame() {
	g.initShop()
	g.World.SetTime(5000)
	for _, t := range g.Data.TeamName {
		g.beds[t] = true
	}
	g.PlacedBlock = map[cube.Pos]bool{}
	g.LastDamager = map[string]string{}
	g.Traps = map[string][]string{}
	for _, pl := range g.Players {
		pl.RemoveScoreboard()
		g.AxeType[pl.Name] = 1
		g.PickaxeType[pl.Name] = 1
		g.setColorTag(pl)
		g.teleportToSpawn(pl)
		pl.ClearInventory()
		pl.SetGameMode("survival")
		g.setArmor(pl)
		g.setSword(pl)
		pl.Title20(util.Msg("messages.fight-title"), "")
		g.Kills[pl.Name] = 0
		g.FinalKills[pl.Name] = 0
		g.BedsBroken[pl.Name] = 0
	}
	g.Phase = PhaseGame
	g.prepareWorld()
}

// prepareWorld resets team upgrades, creates generators and removes teams without players.
func (g *Game) prepareWorld() {
	for _, t := range g.Data.TeamName {
		g.UtilityArena[t] = map[string]int{"generator": 1, "sharpness": 1, "protection": 1, "haste": 1, "health": 1}
	}
	g.initGenerator()
	g.checkTeam()
}

// readBeds samples the bed blocks of every team from the world. Call it without holding the plugin
// lock.
func readBeds(w *world.World, pos map[string]cube.Pos) map[string]bool {
	res := make(map[string]bool, len(pos))
	if w == nil || len(pos) == 0 {
		return res
	}
	done := w.Exec(func(tx *world.Tx) {
		for team, p := range pos {
			_, ok := tx.Block(p).(block.Bed)
			res[team] = ok
		}
	})
	// A stalled world tick must not hold up the arena loop indefinitely.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}
	return res
}


package game

import (
	"strconv"
	"time"

	"github.com/df-mc/dragonfly/server/block/cube"

	"github.com/frogcreative/bedwars/internal/arena"
	"github.com/frogcreative/bedwars/internal/cfg"
	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// Timers holds the match countdowns, in seconds.
type Timers struct {
	// WaitTime is the lobby countdown.
	WaitTime        int
	// UpgradeNextTime is the next generator upgrade stage (1-4); higher values mean the final
	// countdown is running.
	UpgradeNextTime int
	UpgradeTime     int
	// GameOverTime is the final countdown; the match ends when it reaches zero.
	GameOverTime    int
	// RestartTime is the countdown before the arena resets.
	RestartTime     int
}

// reload resets every countdown from the configuration.
func (t *Timers) reload(g *Game) {
	if arena.Str(g.Data.World) == "" {
		return
	}
	t.WaitTime = cfg.Int("game.timers.lobby-countdown", 20)
	t.UpgradeNextTime = 1
	t.UpgradeTime = cfg.Int("game.timers.upgrade-interval", 300)
	t.GameOverTime = cfg.Int("game.timers.game-over-time", 600)
	t.RestartTime = cfg.Int("game.timers.restart-time", 10)
}

// cfgStr reads a string from the configuration.
func cfgStr(key string) string { return cfg.String(key, "") }

// mapValues returns the values of m.
func mapValues(m map[string]*plat.Player) []*plat.Player {
	out := make([]*plat.Player, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

// run ticks the arena once per second until it is stopped.
func (g *Game) run() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-g.stop:
			return
		case <-t.C:
			g.tick()
		}
	}
}

// tick samples world state without the lock, then runs one logic step with the lock held.
func (g *Game) tick() {
	// Sample world state first, without holding the lock: world reads wait for the world goroutine,
	// which may itself be waiting for the lock.
	g.P.Mu.Lock()
	w, phase, setup := g.World, g.Phase, g.Setup
	var bedPos map[string]cube.Pos
	if !setup && phase == PhaseGame {
		bedPos = map[string]cube.Pos{}
		for _, team := range g.Data.TeamName {
			if s, ok := g.Data.TeamBed[team]; ok {
				v := util.MustVec(s).Floor()
				bedPos[team] = cube.Pos{int(v.X), int(v.Y), int(v.Z)}
			}
		}
	}
	g.P.Mu.Unlock()
	var beds map[string]bool
	if len(bedPos) > 0 {
		beds = readBeds(w, bedPos)
	}

	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	// The snapshot may only flip a bed from alive to destroyed; the reverse would resurrect beds that
	// were just broken.
	for team, alive := range beds {
		if !alive {
			g.beds[team] = false
		}
	}
	g.taskRun()
}

// taskRun advances the arena by one second: lobby countdown, match timers, generators, respawns,
// scoreboards and win checks, or the restart sequence.
func (g *Game) taskRun() {
	if g.Setup || g.World == nil {
		return
	}
	text := util.Msg("scoreboard.title")
	serverIP := cfg.String("game.server-ip", "play.youripaddress.com")
	t := &g.T

	switch g.Phase {
	case PhaseLobby:
		if len(g.Players) >= 2*g.Data.Max() {
			if t.WaitTime > 0 {
				t.WaitTime--
				for _, pl := range g.Players {
					if !pl.Online() {
						return
					}
					pl.SetScoreboard(text, g.lobbyLines(util.Msg("scoreboard.lobby-lines.starting-in", "time", t.WaitTime), serverIP))
				}
			}
			for _, k := range cfg.Keys("messages.countdown") {
				if n, err := strconv.Atoi(k); err == nil && t.WaitTime == n {
					g.broadcast(util.Msg("messages.countdown." + k))
				}
			}
			if t.WaitTime <= 5 && t.WaitTime >= 1 {
				for _, pl := range g.Players {
					pl.Sound("random.toast")
					c := "§e"
					if t.WaitTime <= 3 {
						c = "§c"
					}
					pl.Title20(c+strconv.Itoa(t.WaitTime), "")
				}
			}
			if t.WaitTime == 0 {
				g.startGame()
				for _, pl := range g.Players {
					for _, line := range util.MsgList("messages.game-start-banner") {
						pl.Msg(line)
					}
					if g.Data.Max() == 1 {
						pl.Msg(util.Msg("messages.solo-no-teaming"))
					}
				}
			}
		} else {
			for _, pl := range g.Players {
				if !pl.Online() {
					return
				}
				pl.SetScoreboard(text, g.lobbyLines(util.Msg("scoreboard.lobby-lines.waiting"), serverIP))
				t.WaitTime = cfg.Int("game.timers.lobby-countdown", 20)
			}
		}

	case PhaseGame:
		events := ""
		if t.UpgradeNextTime <= 4 {
			t.UpgradeTime--
			keys := map[int]string{1: "diamond-2-in", 2: "emerald-2-in", 3: "diamond-3-in", 4: "emerald-3-in"}
			events = util.Msg("scoreboard.game-lines."+keys[t.UpgradeNextTime], "time", util.CalculateTime(t.UpgradeTime))
			if t.UpgradeTime == 0 {
				t.UpgradeTime = cfg.Int("game.timers.upgrade-interval", 300)
				type up struct {
					key, kind string
					level     int
				}
				ups := map[int]up{1: {"diamond-2", "diamond", 2}, 2: {"emerald-2", "emerald", 2}, 3: {"diamond-3", "diamond", 3}, 4: {"emerald-3", "emerald", 3}}
				u := ups[t.UpgradeNextTime]
				g.broadcast(util.Msg("messages.upgrade." + u.key))
				g.upgradeGeneratorTier(u.kind, u.level)
				t.UpgradeNextTime++
			}
		} else {
			t.GameOverTime--
			events = util.Msg("scoreboard.game-lines.game-end-in", "time", util.CalculateTime(t.GameOverTime))
			if t.GameOverTime <= 0 {
				g.endByTimeout()
				return
			}
		}

		g.tickGens()

		for _, r := range append([]*plat.Player(nil), g.Players...) {
			if c, ok := g.RespawnC[r.Name]; ok {
				if c <= 1 {
					delete(g.RespawnC, r.Name)
					r.Title20(util.Msg("messages.respawned-title"), "")
					g.respawn(r)
				} else {
					g.RespawnC[r.Name] = c - 1
					sub := util.Msg("messages.respawn-subtitle", "seconds", g.RespawnC[r.Name])
					r.Subtitle(sub)
					r.Msg(sub)
				}
			}
		}

		for _, m := range g.Players {
			if c, ok := g.IfMilk[m.Name]; ok {
				if c <= 0 {
					delete(g.IfMilk, m.Name)
				} else {
					g.IfMilk[m.Name] = c - 1
				}
			}
		}

		for _, pt := range g.Players {
			team := g.getTeam(pt)
			ua, ok := g.UtilityArena[team]
			if !ok {
				continue
			}
			if ua["haste"] > 1 {
				pt.AddEffect("haste", ua["haste"]-1, 3*time.Second, false)
			}
			if ua["health"] > 1 {
				if sp, ok := g.Data.TeamSpawn[team]; ok {
					if pos, ok := g.P.Pos[pt.Name]; ok && pos.Distance(util.MustVec(sp)) <= 8 {
						pt.AddEffect("regeneration", 1, 3*time.Second, false)
					}
				}
			}
		}

		all := append(append([]*plat.Player(nil), g.Players...), mapValues(g.Spectators)...)
		date := time.Now().Format("01/02/06")
		for _, pl := range all {
			pl.HealthTag(cfg.String("scoreboard.unicode.health-symbol", ""))
			team := g.getTeam(pl)
			if !pl.Online() || team == "" {
				continue
			}
			pl.SetFood(20)
			lines := []string{"§7" + date, " ", events, "  "}
			for _, tn := range g.Data.TeamName {
				you := ""
				if tn == team {
					you = "§7YOU"
				}
				lines = append(lines, "§l"+g.Data.TeamColor[tn]+util.Ucfirst(tn[:1])+" §r§f"+util.Ucfirst(tn)+": "+g.statusTeam(tn)+" "+you)
			}
			lines = append(lines,
				"       ",
				util.Msg("scoreboard.game-lines.kills", "kills", g.Kills[pl.Name]),
				util.Msg("scoreboard.game-lines.final-kills", "kills", g.FinalKills[pl.Name]),
				util.Msg("scoreboard.game-lines.beds-broken", "count", g.BedsBroken[pl.Name]),
				"   ",
				"§e"+serverIP)
			pl.SetScoreboard(text, lines)
		}

		if alive := g.getAliveTeams(); len(alive) == 1 {
			g.setTeamsWin(alive[0])
		}

	case PhaseRestart:
		t.RestartTime--
		switch t.RestartTime {
		case 0:
			for _, pl := range g.worldPlayers() {
				g.disconnectPlayer(pl, "", false)
				pl.RemoveScoreboard()
			}
		case -1:
			if w, err := g.P.Worlds.LoadMap(arena.Str(g.Data.World)); err != nil {
				g.P.Log.Error(err.Error())
			} else {
				g.World = w
			}
		case -6:
			g.loadArena(true)
			g.T.reload(g)
			g.destroyEntity()
		}
	}
}

// lobbyLines builds the lobby scoreboard.
func (g *Game) lobbyLines(statusLine, serverIP string) []string {
	return []string{
		" ",
		util.Msg("scoreboard.lobby-lines.map", "map", arena.Str(g.Data.DisplayName)),
		util.Msg("scoreboard.lobby-lines.players", "count", len(g.Players), "max", g.maxPlayers()),
		"  ",
		statusLine,
		"   ",
		util.Msg("scoreboard.lobby-lines.mode", "mode", util.MaxInTeamToGroup(g.Data.Max())),
		"    ",
		"§e" + serverIP,
	}
}

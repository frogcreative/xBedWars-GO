package game

import (
	"strings"

	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/player/form"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// Ops lists the players allowed to use /bw and operator chat commands (lower-case names).
var Ops = map[string]bool{}

// IsOp reports whether the player name is listed as an operator.
func IsOp(name string) bool { return Ops[strings.ToLower(name)] }

// Chat routes a chat message sent by a player inside this arena: everybody in the lobby,
// spectators among themselves, the whole arena in solo mode, and the team (or the arena with a "!"
// shout prefix) in team modes. The caller cancels the default broadcast.
func (g *Game) Chat(pl *plat.Player, msg string) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()

	team := g.getTeam(pl)
	if g.Phase == PhaseLobby || g.Phase == PhaseRestart {
		for _, p := range g.worldPlayers() {
			p.Msg(pl.Name + " §7: " + msg)
		}
		return
	}
	if _, spectating := g.Spectators[pl.Name]; spectating {
		for _, s := range g.Spectators {
			s.Msg("§7[SPECTATOR] §r" + pl.Name + ": §7" + msg)
		}
		return
	}
	if msg == "!reduceTime" && IsOp(pl.Name) {
		g.reduceTime(pl)
		return
	}
	teamColor := g.Data.TeamColor[team]
	if g.Data.Max() == 1 {
		g.broadcast(util.Msg("messages.team-chat", "teamcolor", teamColor, "player", pl.Name, "message", msg))
		return
	}
	if strings.HasPrefix(msg, "!") && msg != "!reduceTime" {
		if shout := msg[1:]; strings.TrimSpace(shout) != "" {
			g.broadcast(util.Msg("messages.shout", "teamcolor", teamColor, "team", team, "player", pl.Name, "message", shout))
		}
		return
	}
	for _, mate := range g.Players {
		if g.getTeam(mate) == team {
			mate.Msg(teamColor + "TEAM > " + pl.Name + ": §7" + msg)
		}
	}
}

// OpenSpectatorMenu shows the list of living players a spectator can teleport to. Call it from
// inside the spectator transaction.
func (g *Game) OpenSpectatorMenu(p *player.Player) {
	g.P.Mu.Lock()
	// Keep these slices aligned: the form callback resolves its selected button by index.
	var names, texts []string
	for _, pl := range g.Players {
		if _, spectating := g.Spectators[pl.Name]; spectating {
			continue
		}
		names = append(names, pl.Name)
		texts = append(texts, "§7Teleport to "+g.Data.TeamColor[g.getTeam(pl)]+pl.Name)
	}
	g.P.Mu.Unlock()
	if len(names) == 0 {
		return
	}
	texts = append(texts, "§c§lExit")
	viewer := p.Name()
	p.SendForm(newSpectatorMenu(texts, func(_ *player.Player, i int) {
		if i < len(names) {
			g.teleportSpectator(viewer, names[i])
		}
	}))
}

// newSpectatorMenu builds the spectator menu form.
func newSpectatorMenu(texts []string, fn func(p *player.Player, idx int)) form.Menu {
	return newMenu("§7Spectator Menu", "§fSelect a player:", texts, fn)
}

// teleportSpectator moves the viewer to the last known position of the target.
func (g *Game) teleportSpectator(viewer, target string) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	v, t := g.P.Online[viewer], g.P.Online[target]
	if v == nil || t == nil || !g.inGame(t) {
		return
	}
	pos, ok := g.P.Pos[target]
	if !ok {
		return
	}
	v.TeleportTo(g.World, mgl64.Vec3{pos.X, pos.Y, pos.Z})
	v.Msg("§eYou spectator " + target)
}


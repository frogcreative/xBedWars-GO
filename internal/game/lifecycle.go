package game

import (
	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// worldPlayers returns every player currently inside the arena, including spectators and winners.
func (g *Game) worldPlayers() []*plat.Player {
	var out []*plat.Player
	for name, gg := range g.P.InGame {
		if gg == g {
			if pl, ok := g.P.Online[name]; ok {
				out = append(out, pl)
			}
		}
	}
	return out
}

// bedState reports whether the player team bed still stands.
func (g *Game) bedState(pl *plat.Player) bool {
	t := g.getTeam(pl)
	return t != "" && g.beds[t]
}

// safeSpawn returns the world spawn, raised by one block.
func (g *Game) safeSpawn() mgl64.Vec3 {
	sp := g.World.Spawn()
	return mgl64.Vec3{float64(sp[0]) + 0.5, float64(sp[1]) + 1, float64(sp[2]) + 0.5}
}

// bedPos returns the stored bed position of a team.
func (g *Game) bedPos(team string) (cube.Pos, bool) {
	s, ok := g.Data.TeamBed[team]
	if !ok {
		return cube.Pos{}, false
	}
	v := util.MustVec(s).Floor()
	return cube.Pos{int(v.X), int(v.Y), int(v.Z)}, true
}

// removeBedAt deletes both halves of the bed at p. It must run on the world goroutine.
func removeBedAt(tx *world.Tx, p cube.Pos) bool {
	b, ok := tx.Block(p).(block.Bed)
	if !ok {
		return false
	}
	other := p.Side(b.Facing.Face())
	if b.Head {
		other = p.Side(b.Facing.Opposite().Face())
	}
	tx.SetBlock(p, block.Air{}, nil)
	tx.SetBlock(other, block.Air{}, nil)
	return true
}

// removeBedAsync schedules the removal of a bed.
func (g *Game) removeBedAsync(p cube.Pos) {
	g.World.Exec(func(tx *world.Tx) { removeBedAt(tx, p) })
}

// checkTeam removes the bed and generators of teams that start without players.
func (g *Game) checkTeam() {
	for _, team := range g.Data.TeamName {
		if g.getCountTeam(team) == 0 {
			if p, ok := g.bedPos(team); ok {
				g.removeBedAsync(p)
			}
			g.beds[team] = false
			g.closeTeamGenerators(team)
		}
	}
}

// breakBed destroys a team bed, announces it and credits the breaker.
func (g *Game) breakBed(team string, breaker *plat.Player) {
	p, ok := g.bedPos(team)
	if !ok {
		return
	}
	g.removeBedAsync(p)
	g.beds[team] = false

	if breaker != nil {
		g.broadcast("")
		g.broadcast(util.Msg("messages.bed-destroyed",
			"teamcolor", g.Data.TeamColor[team],
			"team", util.Ucfirst(team),
			"playercolor", g.Data.TeamColor[g.getTeam(breaker)],
			"player", breaker.Name))
		g.BedsBroken[breaker.Name]++
		g.broadcast("")
		for _, e := range g.Players {
			e.Sound("mob.enderdragon.growl")
		}
	}
	for _, pl := range g.Players {
		if g.getTeam(pl) == team {
			pl.Title20(util.Msg("messages.bed-destroyed-title"), util.Msg("messages.no-respawn-subtitle"))
			pl.Sound("mob.enderdragon.growl")
		}
	}
}

// setArmor equips the player with their current armor tier and Protection level.
func (g *Game) setArmor(pl *plat.Player) {
	team := g.getTeam(pl)
	prot := 0
	if lv := g.UtilityArena[team]["protection"]; lv >= 2 && lv <= 5 {
		prot = lv - 1
	}
	tier := g.ArmorType[pl.Name]
	if tier == "" {
		tier = "leather"
	}
	r, gr, b := util.TeamRGB(team)
	pl.SetArmour(tier, r, gr, b, prot)
}

// setSword gives the starting sword, sharpened when the team bought Sharpened Swords.
func (g *Game) setSword(pl *plat.Player) {
	pl.GiveSword(g.UtilityArena[g.getTeam(pl)]["sharpness"] == 2)
}

// startRespawn puts a player in spectator mode for the respawn countdown and lowers their tool
// tiers.
func (g *Game) startRespawn(pl *plat.Player) {
	pl.ClearInventory()
	pl.ClearEffects()
	pl.SetGameMode("spectator")
	pl.TeleportTo(g.World, g.safeSpawn())
	pl.Title20(util.Msg("messages.died-title"), "")
	g.RespawnC[pl.Name] = 6
	g.AxeType[pl.Name] = lessTier(g.AxeType[pl.Name])
	g.PickaxeType[pl.Name] = lessTier(g.PickaxeType[pl.Name])
}

// respawn returns a player to the game with their gear and tools.
func (g *Game) respawn(pl *plat.Player) {
	pl.SetGameMode("survival")
	pl.Title20(util.Msg("messages.respawned-title-ingame"), "")
	pl.Restore()
	pl.StopFlying()
	g.teleportToSpawn(pl)
	g.setArmor(pl)
	g.setSword(pl)
	if t := g.AxeType[pl.Name]; t > 1 {
		pl.ReplaceTool(true, toolStack(true, t, false))
	}
	if t := g.PickaxeType[pl.Name]; t > 1 {
		pl.ReplaceTool(false, toolStack(false, t, false))
	}
}

// startSpectator turns an eliminated player into a spectator and announces the team elimination
// when it was the last member.
func (g *Game) startSpectator(pl *plat.Player) {
	g.removePlayer(pl.Name)
	team := g.getTeam(pl)
	if g.Phase == PhaseGame {
		count := 0
		for _, p := range g.Players {
			if g.getTeam(p) == team {
				if _, sp := g.Spectators[p.Name]; !sp {
					count++
				}
			}
		}
		if count <= 0 {
			g.closeTeamGenerators(team)
			g.broadcast("")
			g.broadcast(util.Msg("messages.team-eliminated", "teamcolor", g.Data.TeamColor[team], "team", team))
			g.broadcast("")
		}
	}
	pl.Title20(util.Msg("messages.died-title"), util.Msg("messages.spectator-subtitle"))
	pl.Sound("mob.wither.death")
	pl.SetScoreTag("")
	pl.SetNameTag(pl.Name)
	g.TempTeam[pl.Name] = team
	g.Spectators[pl.Name] = pl
	delete(g.Teams[team], pl.Name)
	delete(g.ArmorType, pl.Name)
	delete(g.IfShears, pl.Name)
	delete(g.AxeType, pl.Name)
	delete(g.PickaxeType, pl.Name)
	delete(g.RespawnC, pl.Name)
	pl.ClearEffects()
	pl.SetGameMode("spectator")
	pl.Restore()
	pl.StartFlying()
	pl.ClearInventories()
	pl.SetHeldSlot(4)
	sp := g.safeSpawn()
	pl.TeleportTo(g.World, mgl64.Vec3{sp[0], sp[1] + 9, sp[2]})
	pl.ReturnBed()
	pl.SpectatorCompass()
}

// resetRoundState clears the per-round records.
func (g *Game) resetRoundState() {
	g.PlacedBlock = map[cube.Pos]bool{}
	g.UtilityArena = map[string]map[string]int{}
	g.ArmorType = map[string]string{}
	g.AxeType = map[string]int{}
	g.PickaxeType = map[string]int{}
	g.IfShears = map[string]bool{}
	g.Traps = map[string][]string{}
	g.IfMilk = map[string]int{}
}

// setTeamsWin ends the match with the given winning team.
func (g *Game) setTeamsWin(team string) {
	g.destroyEntity()
	for _, p := range g.worldPlayers() {
		p.SetNameTag(p.Name)
		p.SetScoreTag("")
	}
	for _, sp := range mapValues(g.Spectators) {
		g.unsetPlayer(sp)
	}
	for _, pl := range append([]*plat.Player(nil), g.Players...) {
		if g.getTeam(pl) != team {
			continue
		}
		pl.Title20(util.Msg("messages.victory-title"), "")
		pl.Restore()
		pl.TeleportTo(g.World, g.safeSpawn())
		pl.SetGameMode("adventure")
		pl.ClearInventories()
		pl.Sound("random.levelup")
		pl.RemoveScoreboard()
		g.unsetPlayer(pl)
		pl.ReturnBed()
	}
	g.resetRoundState()
	g.Phase = PhaseRestart
}

// setDraw ends the match without a winner.
func (g *Game) setDraw() {
	g.destroyEntity()
	for _, p := range g.worldPlayers() {
		p.SetScoreTag("")
		p.SetNameTag(p.Name)
	}
	for _, pl := range append([]*plat.Player(nil), g.Players...) {
		if !pl.Online() {
			g.Phase = PhaseRestart
			return
		}
		pl.Restore()
		pl.TeleportTo(g.World, g.safeSpawn())
		pl.SetGameMode("adventure")
		pl.ClearInventories()
		pl.RemoveScoreboard()
		g.unsetPlayer(pl)
		pl.ReturnBed()
	}
	g.resetRoundState()
	for _, p := range g.worldPlayers() {
		p.Title20(util.Msg("messages.game-over-title"), "")
	}
	g.Phase = PhaseRestart
}

// endByTimeout ends the match when the final countdown expires. Teams that still own a bed beat
// teams without one; among the remaining candidates the team with the most players wins, and a tie
// is a draw.
func (g *Game) endByTimeout() {
	alive := g.getAliveTeams()
	if len(alive) == 0 {
		g.setDraw()
		return
	}
	candidates := make([]string, 0, len(alive))
	for _, team := range alive {
		if g.beds[team] {
			candidates = append(candidates, team)
		}
	}
	if len(candidates) == 0 {
		candidates = alive
	}
	best := -1
	var winners []string
	for _, team := range candidates {
		switch n := g.getCountTeam(team); {
		case n > best:
			best, winners = n, []string{team}
		case n == best:
			winners = append(winners, team)
		}
	}
	if len(winners) == 1 {
		g.setTeamsWin(winners[0])
		return
	}
	g.setDraw()
}

// reduceTime shortens the current countdown by 50 seconds (operator chat command).
func (g *Game) reduceTime(pl *plat.Player) {
	t := &g.T
	if t.UpgradeNextTime <= 4 {
		if t.UpgradeTime > 70 {
			t.UpgradeTime -= 50
			return
		}
	} else if t.GameOverTime > 70 {
		t.GameOverTime -= 50
		return
	}
	pl.Msg("§cPlease wait to reduce time again!")
}

// BreakCheck applies the block-breaking rules. It returns whether to cancel the break and whether
// to suppress the drops.
func (g *Game) BreakCheck(pl *plat.Player, pos, other cube.Pos, isBed bool) (cancel, noDrops bool) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	if _, sp := g.Spectators[pl.Name]; sp {
		return true, false
	}
	if g.Phase == PhaseLobby || g.Phase == PhaseRestart {
		return true, false
	}
	if isBed {
		for _, team := range g.Data.TeamName {
			bp, ok := g.bedPos(team)
			if !ok || (bp != pos && bp != other) {
				continue
			}
			if g.getTeam(pl) != team {
				g.breakBed(team, pl)
				return false, true
			}
			pl.Msg("§cYou can't break bed your team")
			return true, false
		}
		return false, false
	}
	if !g.PlacedBlock[pos] {
		pl.Msg("§cYou can't break block in here")
		return true, false
	}
	return false, false
}

// PlaceCheck applies the block-placing rules. handled is true for TNT and tower chests, which the
// caller turns into their special behavior instead of a normal placement.
func (g *Game) PlaceCheck(pl *plat.Player, pos cube.Pos, special string) (cancel, handled bool) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	if _, sp := g.Spectators[pl.Name]; sp {
		return true, false
	}
	if g.Phase == PhaseLobby || g.Phase == PhaseRestart {
		return true, false
	}
	bp := util.Vec{X: float64(pos[0]), Y: float64(pos[1]), Z: float64(pos[2])}
	for _, s := range g.Data.TeamSpawn {
		if bp.Distance(util.MustVec(s)) < 8 {
			pl.Msg("§cYou can't place blocks in here.")
			return true, false
		}
	}
	for _, list := range g.Data.Generator {
		for _, s := range list {
			if bp.Distance(util.MustVec(s)) < 1 {
				pl.Msg("§cYou can't place blocks in here.")
				return true, false
			}
		}
	}
	if pos[1] > 256 {
		pl.Msg("§cYou have reached the build limit.")
		return true, false
	}
	if special != "" {
		return true, true
	}
	g.PlacedBlock[pos] = true
	return false, false
}

// ItemDropAllowed reports whether the player may drop the item. Equipment cannot be dropped.
func (g *Game) ItemDropAllowed(pl *plat.Player, isTool bool) bool {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	if _, sp := g.Spectators[pl.Name]; sp {
		return false
	}
	if g.Phase == PhaseLobby || g.Phase == PhaseRestart {
		return false
	}
	return !isTool
}

// AttackAllowed reports whether the attacker may hit the victim. Teammates, spectators and
// respawning players cannot attack.
func (g *Game) AttackAllowed(attacker, victim string) bool {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	if g.Phase != PhaseGame {
		return false
	}
	a, ok1 := g.P.Online[attacker]
	v, ok2 := g.P.Online[victim]
	if !ok1 || !ok2 {
		return true
	}
	if _, sp := g.Spectators[attacker]; sp {
		return false
	}
	if _, rs := g.RespawnC[attacker]; rs {
		return false
	}
	return g.getTeam(a) != g.getTeam(v)
}

// Hurt applies the damage rules and detects deaths. It returns whether to cancel the damage and
// whether the death is final; for a final death the caller drops the victim items.
func (g *Game) Hurt(pl *plat.Player, cause, attacker string, health, damage float64) (cancel, final bool) {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	if c, ok := g.CauseOverride[pl.Name]; ok {
		cause = c
		delete(g.CauseOverride, pl.Name)
	}
	switch g.Phase {
	case PhaseLobby, PhaseRestart:
		return true, false
	}
	if _, sp := g.Spectators[pl.Name]; sp {
		return true, false
	}
	if _, rs := g.RespawnC[pl.Name]; rs {
		return true, false
	}
	if attacker != "" {
		if a, ok := g.P.Online[attacker]; ok {
			if g.getTeam(a) == g.getTeam(pl) {
				return true, false
			}
			g.LastDamager[pl.Name] = attacker
		}
	}
	// Lethal damage (or any void damage) is cancelled and converted into a respawn or a final kill.
	if cause != "void" && health-damage > 0 {
		return false, false
	}
	final = !g.bedState(pl)
	g.onDeath(pl, cause, final)
	return true, final
}

// onDeath handles a death: final kill or respawn, kill credit and the death message.
func (g *Game) onDeath(pl *plat.Player, cause string, final bool) {
	team := g.getTeam(pl)
	tc := g.Data.TeamColor[team]
	killer := g.LastDamager[pl.Name]
	suffix := ""
	if final {
		suffix = "§l§bFINAL KILL!"
		g.startSpectator(pl)
		if killer != "" {
			g.FinalKills[killer]++
			if k, ok := g.P.Online[killer]; ok {
				k.Sound("random.levelup")
			}
		}
	} else {
		g.startRespawn(pl)
		if killer != "" {
			g.Kills[killer]++
			if k, ok := g.P.Online[killer]; ok {
				k.Sound("random.orb")
			}
		}
	}

	kc := ""
	if killer != "" {
		if k, ok := g.P.Online[killer]; ok {
			kc = g.Data.TeamColor[g.getTeam(k)]
		}
	}
	name := tc + pl.Name
	msg := ""
	switch cause {
	case "attack":
		if killer != "" {
			msg = name + " §7was killed by " + kc + killer + suffix
		}
	case "projectile":
		if killer != "" {
			msg = name + " §7was killed by " + kc + killer + " §7with projectile. " + suffix
		} else {
			msg = name + " §7death with projectile. " + suffix
		}
	case "block_explosion":
		if killer != "" {
			msg = name + " §7death with explosion by " + kc + killer + "§7. " + suffix
		} else {
			msg = name + " §7death by explosion. " + suffix
		}
	case "entity_explosion":
		if killer != "" {
			msg = name + " §7was exploded by " + kc + killer + "§7. " + suffix
		} else {
			msg = name + " §7death by explosion. " + suffix
		}
	case "fall":
		if killer != "" {
			msg = name + " §7fell from high place by " + kc + killer + "§7. " + suffix
		} else {
			msg = name + " §7fell from high place. " + suffix
		}
	case "void":
		if killer != "" {
			msg = name + " §7was thrown into void by §f" + kc + killer + "§7. " + suffix
		} else {
			msg = name + " §7fell into void. " + suffix
		}
	default:
		msg = name + " §7death. " + suffix
	}
	g.broadcast(msg)
	delete(g.LastDamager, pl.Name)
}

// Playing reports whether the player is actively playing the running match.
func (g *Game) Playing(name string) bool {
	g.P.Mu.Lock()
	defer g.P.Mu.Unlock()
	_, sp := g.Spectators[name]
	_, rs := g.RespawnC[name]
	return g.Phase == PhaseGame && !sp && !rs
}

// DrinkMilk disables traps for the player for 30 seconds.
func (g *Game) DrinkMilk(name string) {
	g.P.Mu.Lock()
	g.IfMilk[name] = 30
	g.P.Mu.Unlock()
}

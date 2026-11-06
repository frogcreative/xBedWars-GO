package cmds

import (
	"strconv"
	"strings"
	"time"

	"github.com/go-gl/mathgl/mgl64"

	"github.com/frogcreative/bedwars/internal/arena"
	"github.com/frogcreative/bedwars/internal/game"
	"github.com/frogcreative/bedwars/internal/util"
)

// bullet prefixes help entries.
const bullet = "§8§l• §r§7"

// vec3 converts a util.Vec to a Dragonfly vector.
func vec3(v util.Vec) mgl64.Vec3 { return mgl64.Vec3{v.X, v.Y, v.Z} }

// colorList lists the available team colors.
func colorList() string {
	parts := make([]string, 0, len(util.TeamColorOrder))
	for _, n := range util.TeamColorOrder {
		parts = append(parts, "§l"+util.TeamColor[n]+n)
	}
	return strings.Join(parts, "§r§7, ")
}

// arenaOrErr resolves the arena named by the first argument.
func arenaOrErr(c *Ctx, a []string) (*game.Game, string, bool) {
	if len(a) == 0 {
		return nil, "", false
	}
	g, ok := game.P.Arenas[a[0]]
	if !ok {
		c.msg("§cArena " + a[0] + " was not found!")
		return nil, a[0], false
	}
	return g, a[0], true
}

// getNearestTeam returns the team whose spawn is closest to the caller within 17 blocks.
func getNearestTeam(c *Ctx) string {
	g := c.setter()
	if g == nil {
		return ""
	}
	found, dist := "", 100.0
	for _, team := range g.Data.TeamName {
		sp, ok := g.Data.TeamSpawn[team]
		if !ok {
			continue
		}
		d := c.Pos.Distance(util.MustVec(sp))
		if d <= 17 && d < dist {
			dist, found = d, team
		}
	}
	return found
}

// help prints the command list, or the setup checklist during a setup session.
func help(c *Ctx) {
	g := c.setter()
	if g == nil {
		c.msg("§aCommands")
		c.Pl.Msg(bullet + "/newbedwars join <worldName>")
		c.Pl.Msg(bullet + "/newbedwars setupArena <worldName>")
		c.Pl.Msg(bullet + "/newbedwars enableArena <worldName>")
		c.Pl.Msg(bullet + "/newbedwars disableArena <worldName>")
		c.Pl.Msg(bullet + "/newbedwars deleteArena <worldName>")
		return
	}
	d := g.Data
	var spawnNot, bedNot, shopNot, upNot, genNot string
	for _, t := range d.TeamName {
		col := d.TeamColor[t]
		if _, ok := d.TeamSpawn[t]; !ok {
			spawnNot += col + "▋"
		}
		if _, ok := d.TeamBed[t]; !ok {
			bedNot += col + "▋"
		}
		if _, ok := d.TeamShop[t]; !ok {
			shopNot += col + "▋"
		}
		if _, ok := d.TeamUpgrade[t]; !ok {
			upNot += col + "▋"
		}
		tg := d.TeamGenerator[t]
		_, hasIron := tg["iron"]
		_, hasGold := tg["gold"]
		if !(hasIron && hasGold) {
			genNot += col + "▋"
		}
	}
	strike := func(done bool) string {
		if done {
			return "§m"
		}
		return ""
	}
	state := func(done bool) string {
		if done {
			return "§a(SET)"
		}
		return "§c(NOT SET)"
	}
	allSet := func(rem string) string {
		if rem == "" {
			return "§a(ALL SET)"
		}
		return "§c(Remaining: " + rem + "§r§c)"
	}
	genCounts := "§e(§2E" + strconv.Itoa(len(d.Generator["emerald"])) + " §bD" + strconv.Itoa(len(d.Generator["diamond"])) + "§e)"

	c.Pl.Msg("")
	c.msg("§aSetup Commands")
	c.Pl.Msg(bullet + "/newbedwars " + strike(d.WaitingSpawn != nil) + "setWaitingSpawn§r " + state(d.WaitingSpawn != nil))
	c.Pl.Msg(bullet + "/newbedwars autoCreateTeams §e(auto detect)")
	c.Pl.Msg(bullet + "/newbedwars createTeam <name> <color> §e(" + strconv.Itoa(len(d.TeamName)) + " CREATED)")
	c.Pl.Msg(bullet + "/newbedwars removeTeam <name>")
	if len(d.TeamName) >= 1 {
		c.Pl.Msg(bullet + "/newbedwars " + strike(spawnNot == "") + "setSpawn§r " + allSet(spawnNot))
		c.Pl.Msg(bullet + "/newbedwars " + strike(bedNot == "") + "setBed§r " + allSet(bedNot))
		c.Pl.Msg(bullet + "/newbedwars " + strike(shopNot == "") + "setShop§r " + allSet(shopNot))
		c.Pl.Msg(bullet + "/newbedwars " + strike(upNot == "") + "setUpgrade§r " + allSet(upNot))
		rem := ""
		if genNot != "" {
			rem = "§c(Remaining: " + genNot + "§r§c) "
		}
		c.Pl.Msg(bullet + "/newbedwars addGenerator§r " + rem + genCounts)
	} else {
		c.Pl.Msg(bullet + "/newbedwars setSpawn §c(NOT SET)")
		c.Pl.Msg(bullet + "/newbedwars setBed §c(NOT SET)")
		c.Pl.Msg(bullet + "/newbedwars setShop §c(NOT SET)")
		c.Pl.Msg(bullet + "/newbedwars setUpgrade §c(NOT SET)")
		c.Pl.Msg(bullet + "/newbedwars addGenerator§r " + genCounts)
	}
	c.Pl.Msg(bullet + "/newbedwars " + strike(d.MaxInTeam != nil) + "setType§r " + state(d.MaxInTeam != nil))
	c.Pl.Msg(bullet + "/newbedwars save")
}

// join: /bw join <world>.
func join(c *Ctx, a []string) {
	if len(a) == 0 {
		c.msg("§cUsage: /newbedwars join <worldName>")
		return
	}
	g, _, ok := arenaOrErr(c, a)
	if !ok {
		return
	}
	g.JoinToArena(c.Pl)
}

// setupArena: /bw setupArena <world> starts a setup session in the arena world.
func setupArena(c *Ctx, a []string) {
	if len(a) == 0 {
		return
	}
	name := a[0]
	if c.setter() != nil {
		c.msg("§cYou are already in setup mode!")
		return
	}
	if name != strings.ToLower(name) {
		c.msg("§c" + name + " mustn't contain capital letters! Rename your folder to: §a" + strings.ToLower(name))
		return
	}
	if strings.Index(name, "+") > 0 {
		c.msg("§c" + name + " mustn't contain this symbol: +")
		return
	}
	if !game.P.Worlds.IsGenerated(name) {
		c.msg("§a" + name + " doesn't exist!")
		return
	}
	g, exists := game.P.Arenas[name]
	if !exists {
		g = game.NewSetupGame(game.P, name)
		game.P.Arenas[name] = g
	} else {
		g.Data.Enabled = false
	}
	game.P.Setters[c.Pl.Name] = g
	c.Pl.ClearInventory()
	w, err := game.P.Worlds.Load(name)
	if err != nil {
		c.msg("§c" + err.Error())
		delete(game.P.Setters, c.Pl.Name)
		return
	}
	g.Data.World = &name
	dn := util.Ucfirst(name)
	g.Data.DisplayName = &dn
	sp := w.Spawn()
	c.Pl.TeleportTo(w, mgl64.Vec3{float64(sp[0]) + 0.5, float64(sp[1]), float64(sp[2]) + 0.5})
	c.Pl.SetGameMode("creative")
	c.Pl.AddEffect("speed", 3, 24*time.Hour, true)
	c.Pl.Msg("\n\n")
	for i := 0; i < 10; i++ {
		c.Pl.Msg(" ")
	}
	c.msg("§aYou were teleported to the §e" + name + "§a's spawn.")
	c.Pl.Msg(" ")
	help(c)
}

// enableArena: /bw enableArena <world>.
func enableArena(c *Ctx, a []string) {
	if len(a) == 0 {
		c.msg("§cUsage: /newbedwars enableArena <worldName>")
		return
	}
	g, _, ok := arenaOrErr(c, a)
	if !ok {
		return
	}
	if g.Data.Enabled {
		c.msg("§cThis arena is already enabled!")
		return
	}
	if !g.Enable(false) {
		c.msg("§cSetup is not completed!")
		return
	}
	g.LoadArena(false)
	c.msg("§aEnabling arena...")
}

// disableArena: /bw disableArena <world>.
func disableArena(c *Ctx, a []string) {
	if len(a) == 0 {
		c.msg("§cUsage: /newbedwars enableArena <worldName>")
		return
	}
	g, _, ok := arenaOrErr(c, a)
	if !ok {
		return
	}
	if !g.Data.Enabled {
		c.msg("§cThis arena is already disabled!")
		return
	}
	g.Data.Enabled = false
	c.msg("§aDisable arena...")
}

// deleteArena: /bw deleteArena <world> removes the arena file and sends its players to the default
// world.
func deleteArena(c *Ctx, a []string) {
	g, name, ok := arenaOrErr(c, a)
	if !ok {
		return
	}
	spawn := game.P.DefaultSpawn()
	for _, pl := range g.Players {
		pl.TeleportTo(game.P.DefaultWorld(), spawn)
		delete(game.P.InGame, pl.Name)
	}
	arena.Delete(game.P.DataDir, name)
	g.Delete()
	delete(game.P.Arenas, name)
	c.msg("§aSuccesfully delete arena " + name + "!")
}

// setWaitingSpawn: /bw setWaitingSpawn stores the lobby spawn at the caller position.
func setWaitingSpawn(c *Ctx) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	s := floorVec(c.Pos).String()
	g.Data.WaitingSpawn = &s
	c.msg("§aWaiting spawn set for §e" + arena.Str(g.Data.World) + "§a!")
	autoCreateTeams(c)
}

// autoCreateTeams creates a team for every wool color carried in the inventory.
func autoCreateTeams(c *Ctx) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	c.msg("§aSearching for teams")
	if len(c.Wool) == 0 {
		c.msg("§cNo new teams were found.")
		return
	}
	c.msg("§a§lNEW TEAMS FOUND:")
	for _, tf := range c.Wool {
		en := util.EnName(tf)
		if len(g.Data.TeamName) >= 1 {
			if !contains(g.Data.TeamName, en) {
				c.Pl.Msg("§7§l• " + util.GetChatColor(en) + en)
				g.Data.TeamName = append(g.Data.TeamName, en)
				g.Data.TeamColor[en] = util.GetChatColor(en)
			}
		} else {
			c.Pl.Msg("§8§l• " + util.GetChatColor(en) + en)
			g.Data.TeamName = append(g.Data.TeamName, en)
			g.Data.TeamColor[en] = util.GetChatColor(en)
		}
		help(c)
	}
}

// contains reports whether l holds s.
func contains(l []string, s string) bool {
	for _, v := range l {
		if v == s {
			return true
		}
	}
	return false
}

// remove deletes the first occurrence of s from l.
func remove(l []string, s string) []string {
	out := l[:0]
	removed := false
	for _, v := range l {
		if v == s && !removed {
			removed = true
			continue
		}
		out = append(out, v)
	}
	return out
}

// createTeam: /bw createTeam <name> <color>.
func createTeam(c *Ctx, a []string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	if len(a) < 1 {
		c.msg("§cUsage: /newbedwars createTeam <name> <color>")
		return
	}
	if len(a) < 2 {
		c.msg("§aAvailable colors: ")
		c.Pl.Msg(colorList())
		return
	}
	if _, ok := util.TeamColor[strings.ToUpper(a[1])]; !ok {
		c.msg("§cInvalid color!")
		c.msg("§aAvailable colors: ")
		c.Pl.Msg(colorList())
		return
	}
	if contains(g.Data.TeamName, a[0]) {
		c.msg("§c" + a[0] + " team already exists!")
		return
	}
	g.Data.TeamName = append(g.Data.TeamName, a[0])
	g.Data.TeamColor[util.EnName(a[0])] = util.GetChatColor(util.EnName(a[1]))
	c.msg("§a" + a[0] + " created!")
	help(c)
}

// removeTeam: /bw removeTeam <team>.
func removeTeam(c *Ctx, a []string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	d := g.Data
	if len(a) == 0 {
		c.msg("§cUsage: /newbedwars removeTeam <team>")
		c.teamList(g)
		return
	}
	t := a[0]
	if _, ok := d.TeamColor[t]; !ok {
		c.msg("§cThis team doesn't exist: " + t)
		c.Pl.Title(" ", "§cTeam not found: "+t, 250*time.Millisecond, 2*time.Second, 250*time.Millisecond)
		return
	}
	delete(d.TeamSpawn, t)
	delete(d.TeamBed, t)
	delete(d.TeamGenerator, t)
	delete(d.TeamShop, t)
	c.msg("§aTeam removed: " + d.TeamColor[t] + t)
	c.title("§aTeam removed: "+d.TeamColor[t]+t, 40)
	d.TeamName = remove(d.TeamName, t)
	delete(d.TeamColor, t)
	help(c)
}

// setSpawn: /bw setSpawn <team>. A bed found within 17 blocks is assigned to the team
// automatically.
func setSpawn(c *Ctx, a []string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	d := g.Data
	if len(a) == 0 {
		c.msg("§cUsage: /newbedwars setSpawn <team>")
		c.teamList(g)
		return
	}
	t := a[0]
	if !contains(d.TeamName, t) {
		c.msg("§cThis team doesn't exist!")
		if len(d.TeamName) >= 1 {
			c.teamList(g)
			return
		}
	} else {
		d.TeamSpawn[t] = floorVec(c.Pos).String()
		c.msg("§aSpawn set for: " + d.TeamColor[t] + t)
		if c.BedFound != nil {
			c.Pl.TeleportTo(c.W, vec3(*c.BedFound))
			c2 := *c
			c2.Pos = *c.BedFound
			c2.OnBed = true
			setBed(&c2, []string{t})
			return
		}
		rem := ""
		for _, team := range d.TeamName {
			if _, ok := d.TeamSpawn[team]; !ok {
				rem += d.TeamColor[team] + "▋"
			}
		}
		if rem != "" {
			c.msg("§cRemaining: " + rem)
		}
	}
	help(c)
}

// noNearbyTeam explains that no team spawn is close to the caller.
func noNearbyTeam(c *Ctx, g *game.Game, cmdName string) {
	c.msg("§cCould not find any nearby team.")
	c.msg("§cMake sure you set the team's spawn first!")
	c.msg("§cOr if you set the spawn and it wasn't found automatically try using: /newbedwars " + cmdName + " <team>")
	c.title("§cCould not find any nearby team.", 60)
	c.Pl.Sound("mob.villager.no")
	c.teamList(g)
}

// setBed: /bw setBed [team]. The caller must stand on the bed.
func setBed(c *Ctx, a []string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	d := g.Data
	if len(a) == 0 {
		found := getNearestTeam(c)
		if found == "" {
			noNearbyTeam(c, g, "setBed")
		} else {
			setBed(c, []string{found})
		}
		return
	}
	if !c.OnBed {
		c.msg("§cYou must stay on a bed while using this command!")
		c.title("§cYou must stay on a bed.", 40)
		c.Pl.Sound("mob.villager.no")
		return
	}
	t := a[0]
	if !contains(d.TeamName, t) {
		c.msg("§cThis team doesn't exist!")
		if len(d.TeamName) >= 1 {
			c.teamList(g)
			return
		}
	} else {
		d.TeamBed[t] = floorVec(c.Pos).String()
		c.msg("§aBed set for: " + d.TeamColor[t] + t)
		c.title("§aBed set for: "+d.TeamColor[t]+t, 40)
		c.Pl.Sound("mob.villager.yes")
	}
	help(c)
}

// setNPC stores a shop or upgrade villager position for a team.
func setNPC(c *Ctx, a []string, key, cmdName, okText string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	d := g.Data
	if len(a) == 0 {
		found := getNearestTeam(c)
		if found == "" {
			noNearbyTeam(c, g, cmdName)
		} else {
			setNPC(c, []string{found}, key, cmdName, okText)
		}
		return
	}
	t := a[0]
	if !contains(d.TeamName, t) {
		c.msg("§cThis team doesn't exist!")
		c.Pl.Sound("mob.villager.no")
		if len(d.TeamName) >= 1 {
			c.teamList(g)
			return
		}
	} else {
		pos := floorVec(c.Pos).String()
		if key == "teamShop" {
			d.TeamShop[t] = pos
		} else {
			d.TeamUpgrade[t] = pos
		}
		c.msg("§a" + okText + d.TeamColor[t] + t)
		c.title("§a"+okText+d.TeamColor[t]+t, 40)
		c.Pl.Sound("mob.villager.yes")
	}
	help(c)
}

// setType: /bw setType <solo|doubles|3v3v3v3|4v4v4v4>.
func setType(c *Ctx, a []string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	list := func() {
		c.msg("§aAvailable type: ")
		c.Pl.Msg("§7• §eSolo")
		c.Pl.Msg("§7• §eDoubles")
		c.Pl.Msg("§7• §e3v3v3v3")
		c.Pl.Msg("§7• §e4v4v4v4")
	}
	if len(a) == 0 {
		list()
		return
	}
	n := 0
	switch strings.ToLower(a[0]) {
	case "solo":
		n = 1
	case "doubles":
		n = 2
	case "3v3v3v3":
		n = 3
	case "4v4v4v4":
		n = 4
	}
	if n == 0 {
		list()
		c.Pl.Sound("mob.villager.no")
	} else {
		g.Data.MaxInTeam = &n
		c.msg("§aArena group changed to: §d" + a[0])
		c.Pl.Sound("mob.villager.yes")
	}
	help(c)
}

// addGenerator registers a generator: team iron/gold (standing near a team spawn), or a shared
// diamond/emerald generator.
func addGenerator(c *Ctx, a []string) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	d := g.Data
	pos := floorVec(c.Pos).String()
	setTG := func(team string, kinds ...string) {
		if d.TeamGenerator[team] == nil {
			d.TeamGenerator[team] = arena.Map[string]{}
		}
		for _, k := range kinds {
			d.TeamGenerator[team][k] = pos
		}
	}
	yes := func(text string) {
		c.msg(text)
		c.title(text, 40)
		c.Pl.Sound("mob.villager.yes")
		help(c)
	}
	noTeam := func(t string) {
		c.msg("§cCould not find team: " + t)
		c.msg("§cUse: /newbedwars createTeam if you want to create one.")
		c.teamList(g)
		c.title("§cCould not find any nearby team.", 60)
		c.Pl.Sound("mob.villager.no")
	}

	// Without an argument, configure the nearest team's generators; if none is nearby, a gem block
	// underfoot identifies a shared diamond or emerald generator.
	if len(a) == 0 {
		team := getNearestTeam(c)
		if team == "" {
			switch c.BlockBelow {
			case "diamond":
				addGenerator(c, []string{"diamond"})
				return
			case "emerald":
				addGenerator(c, []string{"emerald"})
				return
			}
			c.msg("§cCould not find any nearby team.")
			c.msg("§cMake sure you set the team's spawn first!")
			c.msg("§cOr if you set the spawn and it wasn't found automatically try using: /newbedwars addGenerator <team>")
			c.msg("§cOther use: /newbedwars addGenerator <emerald/diamond>")
			c.title("§cCould not find any nearby team.", 60)
			c.Pl.Sound("mob.villager.no")
			return
		}
		setTG(team, "iron", "gold", "emerald")
		yes("§aGenerator set for team: " + d.TeamColor[team] + team)
		return
	}

	typ := strings.ToLower(a[0])
	switch {
	case typ == "diamond" || typ == "emerald":
		for _, p := range d.Generator[typ] {
			if p == pos {
				c.msg("§cThis generator was already set!")
				c.title("§cThis generator was already set!", 30)
				c.Pl.Sound("mob.villager.no")
				return
			}
		}
		d.Generator[typ] = append(d.Generator[typ], pos)
		yes("§a" + util.Ucfirst(typ) + " generator was added!")
	case typ == "iron" || typ == "gold":
		team := ""
		if len(a) < 2 {
			team = getNearestTeam(c)
		} else {
			team = a[1]
			if _, ok := d.TeamColor[team]; !ok {
				noTeam(team)
				return
			}
		}
		if team == "" {
			c.msg("§cCould not find any nearby team.")
			c.Pl.Msg("Try using: /newbedwars addGenerator <iron/gold/upgrade> <team>")
			return
		}
		setTG(team, typ)
		yes("§a" + util.Ucfirst(typ) + " generator added for team: " + util.GetChatColor(team) + team)
	case len(a) >= 2:
		team := a[1]
		if _, ok := d.TeamColor[team]; !ok {
			noTeam(team)
			return
		}
		setTG(team, "iron", "gold")
		yes("§aGenerator set for team: " + d.TeamColor[team] + team)
	}
}

// save finishes the setup: it moves the caller out, snapshots the map, saves the arena file and
// reloads the arena.
func save(c *Ctx) {
	g, ok := c.need(true)
	if !ok {
		return
	}
	g.Enable(false)
	name := arena.Str(g.Data.World)
	pl := c.Pl
	delete(game.P.Setters, pl.Name)
	pl.TeleportTo(game.P.DefaultWorld(), game.P.DefaultSpawn())
	pl.ClearEffects()

	// Sync and world persistence may block, so keep them out of command dispatch.
	go func() {
		pl.Sync()
		if game.P.Worlds.IsGenerated(name) {
			game.P.Worlds.Unload(name)
			if err := game.P.Worlds.SaveMap(name); err != nil {
				pl.Msg(game.Prefix + "§cFailed to save map: " + err.Error())
			}
		}
		game.P.Mu.Lock()
		defer game.P.Mu.Unlock()
		if err := arena.Save(game.P.DataDir, name, g.Data); err != nil {
			pl.Msg(game.Prefix + "§cFailed to save arena: " + err.Error())
		}
		g.ResetWorld()
		if g.Data.Enabled {
			g.LoadArena(false)
		}
		pl.Msg(game.Prefix + "§aArena changes saved!")
		pl.Msg(game.Prefix + "§aYou can now enable it using:")
		pl.Msg(game.Prefix + "§eType: /newbedwars enableArena " + name)
	}()
}

// testJoin: /bw test <world> joins every online player to the arena.
func testJoin(c *Ctx, a []string) {
	if len(a) == 0 {
		c.msg("§cUsage: /newbedwars test <worldName>")
		return
	}
	g, _, ok := arenaOrErr(c, a)
	if !ok {
		return
	}
	for _, pl := range game.P.Online {
		g.JoinToArena(pl)
	}
}

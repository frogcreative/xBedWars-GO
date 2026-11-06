// Package cmds implements the /newbedwars command (aliases nbw, bedwars, bw) and its setup
// subcommands.
//
// Dragonfly has no Commando-style subcommand framework, so one command is registered with two
// runnables: no arguments (help) and free-form arguments that are split and dispatched here.
// Subcommand names are case-insensitive.
package cmds

import (
	"math"
	"strings"
	"time"

	"github.com/df-mc/dragonfly/server/cmd"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/world"

	"github.com/frogcreative/bedwars/internal/game"
	"github.com/frogcreative/bedwars/internal/plat"
	"github.com/frogcreative/bedwars/internal/util"
)

// Root is /bw without arguments and shows the help.
type Root struct{}

// Sub is /bw <subcommand> [arguments...].
type Sub struct {
	Args cmd.Varargs `cmd:"args"`
}

func (Root) Allow(src cmd.Source) bool { return allow(src) }
func (Sub) Allow(src cmd.Source) bool  { return allow(src) }

// allow restricts the command to operators.
func allow(src cmd.Source) bool {
	if p, ok := src.(*player.Player); ok {
		return game.IsOp(p.Name())
	}
	return false
}

func (Root) Run(src cmd.Source, o *cmd.Output, tx *world.Tx) {
	p, ok := src.(*player.Player)
	if !ok {
		return
	}
	ctx := newCtx(p, tx, "help")
	go run(ctx, "help", nil)
}

// Run dispatches the subcommand on its own goroutine, after capturing the data that can only be
// read inside the transaction.
func (s Sub) Run(src cmd.Source, o *cmd.Output, tx *world.Tx) {
	p, ok := src.(*player.Player)
	if !ok {
		return
	}
	fields := strings.Fields(string(s.Args))
	if len(fields) == 0 {
		go run(newCtx(p, tx, "help"), "help", nil)
		return
	}
	name := strings.ToLower(fields[0])
	go run(newCtx(p, tx, name), name, fields[1:])
}

// Ctx carries the data of the calling player that must be captured inside their transaction.
type Ctx struct {
	Pl         *plat.Player
	W          *world.World
	Pos        util.Vec
	BlockBelow string
	OnBed      bool
	BedFound   *util.Vec
	Wool       []string
}

// newCtx captures the calling player data for the named subcommand.
func newCtx(p *player.Player, tx *world.Tx, cmdName string) *Ctx {
	return buildCtx(p, tx, cmdName)
}

// run executes a subcommand under the plugin lock.
func run(c *Ctx, name string, args []string) {
	game.P.Mu.Lock()
	defer game.P.Mu.Unlock()
	dispatch(c, name, args)
}

// msg sends a prefixed message to the caller.
func (c *Ctx) msg(s string) { c.Pl.Msg(game.Prefix + s) }

// setter returns the arena the caller is setting up.
func (c *Ctx) setter() *game.Game { return game.P.Setters[c.Pl.Name] }

// need returns the arena being set up, or tells the caller they are not in a setup session.
func (c *Ctx) need(perm bool) (*game.Game, bool) {
	_ = perm
	g := c.setter()
	if g == nil {
		c.msg("§cYou're not in a setup session!")
		return nil, false
	}
	return g, true
}

// teamList prints the teams defined so far.
func (c *Ctx) teamList(g *game.Game) {
	if len(g.Data.TeamName) >= 1 {
		c.msg("§aAvailable teams: ")
		for _, t := range g.Data.TeamName {
			c.Pl.Msg("§7§l• " + g.Data.TeamColor[t] + t)
		}
	}
}

// title shows a subtitle-only title for stayTicks ticks.
func (c *Ctx) title(sub string, stayTicks int) {
	c.Pl.Title(" ", sub, 5*50*time.Millisecond, time.Duration(stayTicks)*50*time.Millisecond, 5*50*time.Millisecond)
}

// floorVec rounds every component down.
func floorVec(v util.Vec) util.Vec { return util.Vec{X: math.Floor(v.X), Y: math.Floor(v.Y), Z: math.Floor(v.Z)} }

// dispatch runs the named subcommand.
func dispatch(c *Ctx, name string, a []string) {
	switch name {
	case "help":
		help(c)
	case "join":
		join(c, a)
	case "setuparena":
		setupArena(c, a)
	case "enablearena":
		enableArena(c, a)
	case "disablearena":
		disableArena(c, a)
	case "deletearena":
		deleteArena(c, a)
	case "setwaitingspawn":
		setWaitingSpawn(c)
	case "autocreateteams":
		autoCreateTeams(c)
	case "createteam":
		createTeam(c, a)
	case "removeteam":
		removeTeam(c, a)
	case "setspawn":
		setSpawn(c, a)
	case "setbed":
		setBed(c, a)
	case "setshop":
		setNPC(c, a, "teamShop", "setShop", "Shop npc set for: ")
	case "setupgrade":
		setNPC(c, a, "teamUpgrade", "setUpgrade", "Upgrade npc set for: ")
	case "settype":
		setType(c, a)
	case "addgenerator":
		addGenerator(c, a)
	case "save":
		save(c)
	case "test":
		testJoin(c, a)
	default:
		help(c)
	}
}

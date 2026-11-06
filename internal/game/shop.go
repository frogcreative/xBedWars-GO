package game

import (
	"strconv"
	"strings"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/enchantment"
	"github.com/df-mc/dragonfly/server/item/potion"
	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/player/form"
	"github.com/df-mc/dragonfly/server/world"

	"github.com/frogcreative/bedwars/internal/util"
)

// ShopCategories lists the item shop categories.
var ShopCategories = []string{"§fBlocks", "§fMelee", "§fArmor", "§fTools", "§fBow & Arrow", "§fPotions", "§fUtility"}

// shopEntry is one purchasable row of the item shop.
type shopEntry struct {
	Name  string
	Cur   string
	Price int
	Stack item.Stack
	Kind  string
	Arg   string
	Max   bool
}

// button returns the form button text.
func (e shopEntry) button() string {
	if e.Max {
		return e.Name + "\n§aMax"
	}
	return e.Name + "\n" + util.CurrencyColor(e.Cur) + strconv.Itoa(e.Price) + " " + e.Cur
}

// colourFor maps a team name to its dye color.
func colourFor(team string) item.Colour {
	switch strings.ToLower(team) {
	case "orange":
		return item.ColourOrange()
	case "magenta":
		return item.ColourMagenta()
	case "aqua":
		return item.ColourLightBlue()
	case "yellow":
		return item.ColourYellow()
	case "green":
		return item.ColourGreen()
	case "pink":
		return item.ColourPink()
	case "gray":
		return item.ColourGrey()
	case "light gray":
		return item.ColourLightGrey()
	case "cyan":
		return item.ColourCyan()
	case "purple":
		return item.ColourPurple()
	case "blue":
		return item.ColourBlue()
	case "brown":
		return item.ColourBrown()
	case "red":
		return item.ColourRed()
	case "black":
		return item.ColourBlack()
	}
	return item.ColourWhite()
}

// toolLore is the price per pickaxe/axe tier.
var toolLore = map[int]struct {
	Cur   string
	Price int
	Tier  string
}{
	1: {"Iron", 10, "I"}, 2: {"Iron", 10, "I"}, 3: {"Iron", 10, "II"},
	4: {"Gold", 3, "III"}, 5: {"Gold", 6, "IV"}, 6: {"Gold", 6, "V"},
}

// toolEff is the Efficiency level per pickaxe/axe tier.
var toolEff = map[int]int{1: 1, 2: 1, 3: 2, 4: 3, 5: 3, 6: 3}

// toolTier returns the material of a pickaxe/axe tier.
func toolTier(t int) item.ToolTier {
	switch t {
	case 1:
		return item.ToolTierWood
	case 2:
		return item.ToolTierStone
	case 3:
		return item.ToolTierIron
	}
	return item.ToolTierDiamond
}

// toolName returns the display name of a pickaxe/axe tier.
func toolName(isAxe bool, t int) string {
	kind := "Pickaxe"
	if isAxe {
		kind = "Axe"
	}
	mat := map[int]string{1: "Wooden", 2: "Stone", 3: "Iron", 4: "Diamond", 5: "Diamond", 6: "Diamond"}[t]
	return "§a" + mat + " " + kind + " (Efficiency " + roman(toolEff[t]) + ")"
}

// toolStack builds the pickaxe or axe of a tier. Shop items carry a display name; respawn items do
// not.
func toolStack(isAxe bool, t int, forShop bool) item.Stack {
	var it world.Item = item.Pickaxe{Tier: toolTier(t)}
	if isAxe {
		it = item.Axe{Tier: toolTier(t)}
	}
	st := item.NewStack(it, 1).WithEnchantments(item.NewEnchantment(enchantment.Efficiency, toolEff[t]))
	if forShop {
		st = st.WithCustomName(toolName(isAxe, t))
	}
	return st
}

// lessTier returns the tier a player falls back to after dying.
func lessTier(t int) int {
	return map[int]int{6: 4, 5: 4, 4: 3, 3: 2, 2: 1, 1: 1}[t]
}

// named sets the custom name of a stack.
func named(st item.Stack, name string) item.Stack { return st.WithCustomName(name) }

// catalog returns the shop rows of a category for the given player and team.
func (g *Game) catalog(cat, player, team string) []shopEntry {
	price := func(key string) (string, int) { return util.ShopPrice(key) }
	e := func(key, name string, st item.Stack) shopEntry {
		cur, p := price(key)
		return shopEntry{Name: name, Cur: cur, Price: p, Stack: named(st, name)}
	}
	col := colourFor(team)
	switch cat {
	case "§fBlocks":
		return []shopEntry{
			e("blocks.wool", "§eWool", item.NewStack(block.Wool{Colour: col}, 16)),
			e("blocks.terracotta", "§eTerracotta", item.NewStack(block.StainedTerracotta{Colour: col}, 16)),
			e("blocks.stained-glass", "§eStained Glass", item.NewStack(block.StainedGlass{Colour: col}, 4)),
			e("blocks.end-stone", "§eEnd Stone", item.NewStack(block.EndStone{}, 12)),
			e("blocks.ladder", "§eLadder", item.NewStack(block.Ladder{}, 16)),
			e("blocks.plank", "§ePlank", item.NewStack(block.Planks{Wood: block.OakWood()}, 16)),
			e("blocks.obsidian", "§eObsidian", item.NewStack(block.Obsidian{}, 4)),
		}
	case "§fMelee":
		sw := func(key, name, arg string, t item.ToolTier) shopEntry {
			x := e(key, name, item.NewStack(item.Sword{Tier: t}, 1))
			x.Kind, x.Arg = "sword", arg
			return x
		}
		stick := item.NewStack(item.Stick{}, 1).WithEnchantments(item.NewEnchantment(enchantment.Knockback, 1))
		return []shopEntry{
			sw("melee.stone-sword", "§eStone Sword", "stone", item.ToolTierStone),
			sw("melee.iron-sword", "§eIron Sword", "iron", item.ToolTierIron),
			sw("melee.diamond-sword", "§eDiamond Sword", "diamond", item.ToolTierDiamond),
			e("melee.knockback-stick", "§eKnockback Stick", stick),
		}
	case "§fArmor":
		ar := func(key, name, arg string) shopEntry {
			x := e(key, name, item.NewStack(item.Boots{Tier: item.ArmourTierLeather{}}, 1))
			x.Kind, x.Arg = "armor", arg
			return x
		}
		return []shopEntry{
			ar("armor.chainmail-set", "§eChainmail Set", "chainmail"),
			ar("armor.iron-set", "§eIron Set", "iron"),
			ar("armor.diamond-set", "§eDiamond Set", "diamond"),
		}
	case "§fTools":
		sh := e("tools.shears", "§eShears", item.NewStack(item.Shears{}, 1))
		sh.Kind = "shears"
		out := []shopEntry{sh}
		for _, isAxe := range []bool{false, true} {
			t := g.PickaxeType[player]
			arg := "pickaxe"
			if isAxe {
				t, arg = g.AxeType[player], "axe"
			}
			if t < 1 {
				t = 1
			}
			lore := toolLore[t]
			out = append(out, shopEntry{
				Name: toolName(isAxe, t), Cur: lore.Cur, Price: lore.Price,
				Stack: toolStack(isAxe, t, false), Kind: "tool", Arg: arg, Max: t >= 6,
			})
		}
		return out
	case "§fBow & Arrow":
		return []shopEntry{
			e("bow.arrow", "§eArrow", item.NewStack(item.Arrow{}, 8)),
			e("bow.bow", "§eBow", item.NewStack(item.Bow{}, 1)),
			e("bow.bow-power", "§eBow (Power I)", item.NewStack(item.Bow{}, 1).WithEnchantments(item.NewEnchantment(enchantment.Power, 1))),
			e("bow.bow-power-punch", "§eBow (Power I, Punch I)", item.NewStack(item.Bow{}, 1).WithEnchantments(
				item.NewEnchantment(enchantment.Power, 1), item.NewEnchantment(enchantment.Punch, 1))),
		}
	case "§fPotions":
		return []shopEntry{
			e("potions.speed", "§eSpeed Potion II (45 seconds)", item.NewStack(item.Potion{Type: potion.StrongSwiftness()}, 1)),
			e("potions.jump", "§eJump Potion III (45 seconds)", item.NewStack(item.Potion{Type: potion.StrongLeaping()}, 1)),
			e("potions.invisibility", "§eInvisibility Potion (30 seconds)", item.NewStack(item.Potion{Type: potion.Invisibility()}, 1)),
		}
	case "§fUtility":
		tnt := "utility.tnt-large"
		if m := g.Data.Max(); m == 1 || m == 2 {
			tnt = "utility.tnt-small"
		}
		return []shopEntry{
			e("utility.golden-apple", "§eGolden Apple", item.NewStack(item.GoldenApple{}, 1)),
			e("utility.bedbug", "§eBedbug", item.NewStack(item.Snowball{}, 1)),
			e("utility.dream-defender", "§eDream Defender", item.NewStack(item.IronNugget{}, 1)),
			e("utility.fireball", "§eFireball", item.NewStack(item.FireCharge{}, 1)),
			e(tnt, "§eTNT", item.NewStack(block.TNT{}, 1)),
			e("utility.ender-pearl", "§eEnder Pearl", item.NewStack(item.EnderPearl{}, 1)),
			e("utility.egg-bridge", "§eEgg Bridge", item.NewStack(item.Egg{}, 1)),
			e("utility.magic-milk", "§eMagic Milk", item.NewStack(item.Bucket{Content: item.MilkBucketContent()}, 1)),
			e("utility.pop-up-tower", "§eCompact pop up tower", item.NewStack(block.NewChest(), 1)),
		}
	}
	return nil
}

// shopForm adapts a callback to a Dragonfly menu form.
type shopForm struct {
	byText map[string]int
	fn     func(p *player.Player, idx int)
}

// Submit implements form.MenuSubmittable.
func (s *shopForm) Submit(sub form.Submitter, pressed form.Button, _ *world.Tx) {
	if i, ok := s.byText[pressed.Text]; ok {
		if p, ok := sub.(*player.Player); ok {
			s.fn(p, i)
		}
	}
}

// newMenu builds a menu form whose buttons invoke fn with the pressed index.
func newMenu(title, body string, texts []string, fn func(p *player.Player, idx int)) form.Menu {
	sf := &shopForm{byText: map[string]int{}, fn: fn}
	btns := make([]form.Button, 0, len(texts))
	for i, t := range texts {
		sf.byText[t] = i
		btns = append(btns, form.NewButton(t, ""))
	}
	return form.NewMenu(sf, title).WithBody(body).WithButtons(btns...)
}

// OpenShop shows the shop; an empty category shows the category list. Call it from inside the
// player transaction.
func (g *Game) OpenShop(p *player.Player, cat string) {
	if cat == "" {
		p.SendForm(newMenu("Item Shop", "", ShopCategories, func(p *player.Player, i int) {
			g.OpenShop(p, ShopCategories[i])
		}))
		return
	}
	g.P.Mu.Lock()
	pl := g.P.Online[p.Name()]
	team := ""
	if pl != nil {
		team = g.getTeam(pl)
	}
	entries := g.catalog(cat, p.Name(), team)
	g.P.Mu.Unlock()

	texts := make([]string, 0, len(entries)+1)
	for _, e := range entries {
		texts = append(texts, e.button())
	}
	texts = append(texts, "§l« Back")
	p.SendForm(newMenu("Item Shop - "+strings.TrimPrefix(cat, "§f"), "", texts, func(p *player.Player, i int) {
		if i >= len(entries) {
			g.OpenShop(p, "")
			return
		}
		g.purchase(p, entries[i])
		g.OpenShop(p, cat)
	}))
}

// isCurrency reports whether it is the item of the currency.
func isCurrency(it world.Item, cur string) bool {
	switch cur {
	case "Diamond":
		_, ok := it.(item.Diamond)
		return ok
	case "Gold":
		_, ok := it.(item.GoldIngot)
		return ok
	case "Emerald":
		_, ok := it.(item.Emerald)
		return ok
	}
	_, ok := it.(item.IronIngot)
	return ok
}

// currencyItem returns the item of a currency.
func currencyItem(cur string) world.Item {
	switch cur {
	case "Diamond":
		return item.Diamond{}
	case "Gold":
		return item.GoldIngot{}
	case "Emerald":
		return item.Emerald{}
	}
	return item.IronIngot{}
}

// countCurrency counts the currency items in the inventory.
func countCurrency(p *player.Player, cur string) int {
	n := 0
	for _, st := range p.Inventory().Slots() {
		if !st.Empty() && isCurrency(st.Item(), cur) {
			n += st.Count()
		}
	}
	return n
}

// pay removes the price from the inventory, or tells the player how much is missing.
func (g *Game) pay(p *player.Player, cur string, price int) bool {
	have := countCurrency(p, cur)
	if have < price {
		p.Message("§cYou don't have enought " + cur + "! Need " + strconv.Itoa(price-have) + " more!")
		return false
	}
	_ = p.Inventory().RemoveItem(item.NewStack(currencyItem(cur), price))
	return true
}

// bought confirms a purchase.
func bought(p *player.Player, name string) { p.Message("§aYou purchased §6" + name) }

// give adds the stack to the inventory, dropping it at the player feet when the inventory is full.
func give(p *player.Player, st item.Stack) {
	if _, err := p.Inventory().AddItem(st); err != nil {
		p.Tx().AddEntity(entityItem(p, st))
	}
}

// hasSword reports whether the inventory holds a sword of the tier.
func hasSword(p *player.Player, t item.ToolTier) bool {
	for _, st := range p.Inventory().Slots() {
		if sw, ok := st.Item().(item.Sword); ok && sw.Tier == t {
			return true
		}
	}
	return false
}

// swordTier maps a shop key to a tool tier.
func swordTier(arg string) item.ToolTier {
	switch arg {
	case "iron":
		return item.ToolTierIron
	case "diamond":
		return item.ToolTierDiamond
	}
	return item.ToolTierStone
}

// armorRank orders the armor sets from weakest to strongest.
var armorRank = map[string]int{"leather": 0, "chainmail": 1, "iron": 2, "diamond": 3}

// purchase buys one shop entry for the player, applying the special rules for swords, shears,
// armor sets and tool tiers.
func (g *Game) purchase(p *player.Player, e shopEntry) {
	name := p.Name()
	g.P.Mu.Lock()
	pl := g.P.Online[name]
	_, spec := g.Spectators[name]
	_, respawning := g.RespawnC[name]
	if pl == nil || g.Phase != PhaseGame || !g.inGame(pl) || spec || respawning {
		g.P.Mu.Unlock()
		return
	}
	team := g.getTeam(pl)
	g.P.Mu.Unlock()

	switch e.Kind {
	case "sword":
		t := swordTier(e.Arg)
		if hasSword(p, t) {
			p.Message("§cYou already purchase this!")
			return
		}
		if !g.pay(p, e.Cur, e.Price) {
			return
		}
		bought(p, e.Name)
		g.P.Mu.Lock()
		sharp := g.UtilityArena[team]["sharpness"] == 2
		shears := g.IfShears[name]
		g.P.Mu.Unlock()
		sw := item.NewStack(item.Sword{Tier: t}, 1)
		if sharp {
			sw = sw.WithEnchantments(item.NewEnchantment(enchantment.Sharpness, 1))
		}
		_ = p.Inventory().SetItem(0, sw)
		if shears && !hasShears(p) {
			_, _ = p.Inventory().AddItem(item.NewStack(item.Shears{}, 1))
		}

	case "shears":
		g.P.Mu.Lock()
		owned := g.IfShears[name]
		g.P.Mu.Unlock()
		if owned {
			return
		}
		if !g.pay(p, e.Cur, e.Price) {
			return
		}
		g.P.Mu.Lock()
		g.IfShears[name] = true
		g.P.Mu.Unlock()
		bought(p, "Shears")
		if !hasShears(p) {
			_, _ = p.Inventory().AddItem(item.NewStack(item.Shears{}, 1))
		}

	case "armor":
		g.P.Mu.Lock()
		cur := g.ArmorType[name]
		g.P.Mu.Unlock()
		if cur == "" {
			cur = "leather"
		}
		if armorRank[e.Arg] <= armorRank[cur] {
			p.Message("§cYou already purchase this!")
			return
		}
		if !g.pay(p, e.Cur, e.Price) {
			return
		}
		g.P.Mu.Lock()
		g.ArmorType[name] = e.Arg
		g.P.Mu.Unlock()
		bought(p, util.Ucfirst(e.Arg)+" set")
		g.P.Mu.Lock()
		if pl2 := g.P.Online[name]; pl2 != nil {
			g.setArmor(pl2)
		}
		g.P.Mu.Unlock()

	case "tool":
		isAxe := e.Arg == "axe"
		g.P.Mu.Lock()
		tiers := g.PickaxeType
		if isAxe {
			tiers = g.AxeType
		}
		t := tiers[name]
		g.P.Mu.Unlock()
		if t < 1 {
			t = 1
		}
		if t >= 6 {
			return
		}
		if !g.pay(p, e.Cur, e.Price) {
			return
		}
		g.P.Mu.Lock()
		tiers[name] = t + 1
		g.P.Mu.Unlock()
		given := toolStack(isAxe, t, false)
		bought(p, toolName(isAxe, t))
		inv := p.Inventory()
		for slot, s := range inv.Slots() {
			switch s.Item().(type) {
			case item.Axe:
				if isAxe {
					_ = inv.SetItem(slot, item.Stack{})
				}
			case item.Pickaxe:
				if !isAxe {
					_ = inv.SetItem(slot, item.Stack{})
				}
			}
		}
		_, _ = inv.AddItem(given)

	default:
		if !g.pay(p, e.Cur, e.Price) {
			return
		}
		give(p, e.Stack)
		bought(p, e.Name)
	}
}

// hasShears reports whether the inventory holds shears.
func hasShears(p *player.Player) bool {
	for _, st := range p.Inventory().Slots() {
		if _, ok := st.Item().(item.Shears); ok {
			return true
		}
	}
	return false
}

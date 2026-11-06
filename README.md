# NewBedwars untuk Dragonfly

Port dari plugin PocketMine-MP 5 "BEDWARS-DONATION" ke Dragonfly (Go).
Module: `github.com/frogcreative/bedwars`

## Build dan jalankan

```
go get github.com/df-mc/dragonfly@master
go mod tidy
gofmt -w .
go build ./...
go run .
```

- Isi `plugin_data/NewBedwars/ops.txt` dengan satu nama pemain per baris. Hanya nama di file itu yang boleh memakai `/bw` dan `!reduceTime`.
- Taruh folder world arena di `worlds/<nama>/` (format LevelDB Bedrock, sama seperti PM5).
- `config.yml` dan file `Arenas/*.yml` dari PM5 dipakai apa adanya (key sama persis).

## Status fitur

Semua fitur sudah diport: command setup, lobby, tim, countdown, scoreboard, generator (iron, gold, diamond, emerald), item shop, team upgrade, trap, bed, respawn, spectator (compass), final kill, chat tim/shout, TNT, fireball, egg bridge, popup tower, potion, magic milk, bedbug dan dream defender.

## Aturan akhir game (perubahan dari PHP)

Sudden death dan EnderDragon **dihapus**. Setelah 4 tahap upgrade generator, hitung mundur `game.timers.game-over-time` berjalan. Saat habis, game langsung selesai:

1. Tim yang bed-nya masih ada menang atas tim yang bed-nya sudah hancur.
2. Di antara kandidat yang tersisa, tim dengan pemain terbanyak menang.
3. Jika jumlahnya sama, hasilnya seri.

Bed tidak lagi dihancurkan otomatis. Key `bed-destroy-time`, `sudden-death-time`, dan pesan sudden death di `config.yml` sudah tidak dipakai.

## Perbedaan dari versi PHP

- Menu Select Team, Item Shop, Upgrades & Traps, dan Spectator berupa form, bukan chest GUI (Dragonfly tidak punya InvMenu).
- NPC shop dan upgrade adalah entity custom (`villager_v2`); bedbug dan dream defender memakai AI buatan sendiri dengan kecepatan dan damage yang sama.
- TNT dan fireball hanya visual; ledakan dihitung sendiri. Telur egg bridge dan snowball bedbug disimulasikan lintasannya.
- Efek sound custom belum ada padanannya di Dragonfly (no-op). Efek invisibility belum menyembunyikan armor.
- Bug PHP yang diperbaiki: arena penuh menerima pemain ekstra, scoreboard berhenti saat ada pemain offline, key generator huruf besar, Chainmail Set tidak bisa dibeli, Sharpened Swords bisa dibeli berulang, Emerald Forge tidak pernah menghasilkan emerald, durasi efek trap 5/15 tick.
- Command debug `!dragon`, `!bedbug`, `!golem`, `!upgradeGenerator`, dan `/bw sudden` dihapus. `!reduceTime` (op) tetap ada.

## Titik yang perlu dicek saat build

Kode ini ditulis tanpa compiler dan tanpa akses ke Dragonfly 26.60, jadi belum pernah di-build. Jika `go build` menampilkan error, hampir pasti hanya perbedaan nama atau signature API di titik berikut:

| File | Yang bergantung pada API Dragonfly |
|---|---|
| `internal/plat/player.go` | `p.H()`, konstanta `effect.*`, `scoreboard` (`WriteString`), `Heal` + `entity.FoodHealingSource`, tipe item armor/tool dan enchantment |
| `internal/worlds/worlds.go` | `world.EntityRegistry`, `world.Config{Entities}`, `mcdb` |
| `internal/events/handler.go` | signature `Handle*` (mis. `HandleMove` dengan `cube.Rotation`), `p.Tx()`, `e.H()`, `p.SetHeldItems`, `item.Weapon`, form dengan struct submittable |
| `internal/game/generators.go` | `entity.NewText`, `entity.NewItem`, `tx.EntitiesWithin`, `tx.Entities`, `entity.ItemBehaviour`, `Ent.SetNameTag` |
| `internal/game/npc.go`, `explosives.go`, `mobs.go` | `world.EntityType` custom, `entity.Config`, `StationaryBehaviourConfig`, `PassiveBehaviourConfig`, `Ent.SetVelocity`, `world.DamageSource` |
| `internal/game/explosives.go`, `bridge.go` | `block.ExplosionDamageSource`, `sound.Explosion`, `particle.HugeExplosion`, `entity.NewEgg`, `entity.NewSnowball` |
| `internal/game/shop.go` | `item.Colour*`, `block.StainedTerracotta`, `potion.*`, `item.IronNugget`, `item.MilkBucketContent`, `item.ToolTier*` |
| `internal/game/lifecycle.go`, `internal/cmds/ctx.go` | `block.Bed{Facing, Head}`, `block.Diamond`, `block.Emerald` |

Semua pemanggilan API pemain dipusatkan di `internal/plat/player.go` supaya perbaikan cukup di satu tempat.

// Command startingdump — слесарная проверка: читает racebps модуля и
// печатает, какие стартовые отряды и бойцы получает каждая раса — с
// чертежом ebps, смещением появления и признаком «мёртвого» старта.
//
// Флаг -scene дополнительно собирает сцену целиком (постройка + бойцы
// слота 1) и печатает, где объекты встали и не упали ли при расстановке.
package main

import (
	"flag"
	"fmt"
	"math"
	"strings"

	"dow-mod-studio/modapi"
	"dow-mod-studio/racebps"
	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/attrib"
)

const dowRoot = `C:\Program Files\Steam\steamapps\common\Dawn of War Definitive Edition`

func main() {
	module := flag.String("module", "DoWDE", "модуль")
	scene := flag.String("scene", "", "путь к .sgb: собрать сцену и показать стартовых бойцов")
	race := flag.String("race", "space_marine_race", "чертеёж расы для -scene")
	flag.Parse()

	modules, err := modapi.DiscoverModules(dowRoot)
	if err != nil {
		fmt.Println("discover:", err)
		return
	}
	target := modapi.FindModule(modules, *module)
	if target == nil {
		fmt.Println("модуль не найден:", *module)
		return
	}
	tree, err := modapi.BuildModTree(target.Path, "English")
	if err != nil {
		fmt.Println("build:", err)
		return
	}
	defer tree.Close()

	if *scene != "" {
		runScene(tree, *scene, *race)
		return
	}
	for folder, caps := range attrib.LoadRaceCaps(tree) {
		fmt.Printf("=== %s (%s)\n", folder, caps.Path)
		for _, squad := range caps.StartingSquads {
			offset := "нет смещения"
			if squad.HasOffset {
				offset = fmt.Sprintf("offset=(%.1f, %.1f, %.1f)", squad.Offset[0], squad.Offset[1], squad.Offset[2])
			}
			fmt.Printf("  отряд  %s  %s\n", squad.Path, offset)
		}
		for _, unit := range attrib.StartingUnits(tree, caps) {
			state := "живой"
			if unit.Dead {
				state = "МЁРТВЫЙ"
			}
			fmt.Printf("  боец   %-70s %s\n", unit.Ebps, state)
		}
	}
}

// runScene собирает сцену со стартовой постройкой и бойцами слота 1 и
// печатает положение объектов игрока: центр, радиус и высоту.
func runScene(tree *modapi.ModTree, scenePath, raceID string) {
	races, err := racebps.Load(tree, nil)
	if err != nil {
		fmt.Println("racebps:", err)
		return
	}
	var chosen *racebps.Race
	for i := range races {
		if races[i].ID == raceID {
			chosen = &races[i]
			break
		}
	}
	if chosen == nil {
		fmt.Println("раса не найдена:", raceID)
		return
	}
	slot := 1
	var builds []sgbview.SlotBuild
	for _, building := range chosen.StartingBuildings {
		if path := chosen.StructurePath(building); path != "" {
			builds = append(builds, sgbview.SlotBuild{Slot: slot, Blueprint: path})
		}
	}
	var units []sgbview.SlotUnit
	for _, caps := range attrib.LoadRaceCaps(tree) {
		if !strings.EqualFold(caps.Path, chosen.Path) {
			continue
		}
		for _, start := range attrib.StartingUnits(tree, caps) {
			units = append(units, sgbview.SlotUnit{
				Slot: slot, Blueprint: start.Ebps,
				Offset: start.Offset, HasOffset: start.HasOffset, Dead: start.Dead,
			})
		}
		break
	}
	fmt.Printf("сцена %s: раса %s, построек %d, бойцов %d\n", scenePath, chosen.ID, len(builds), len(units))

	sc, err := sgbview.LoadWith(tree, scenePath, sgbview.DefaultLimits(), builds, units...)
	if err != nil {
		fmt.Println("LoadWith:", err)
		return
	}
	fmt.Printf("игроков на карте: %d, объектов: %d, пропущено: %d\n", len(sc.Players), len(sc.Objects), len(sc.Skipped))
	for _, p := range sc.Players {
		fmt.Printf("  слот %d: позиций %d, первая (%.1f, %.1f, %.1f)\n", p.Owner, len(p.Positions), p.Positions[0].X, p.Positions[0].Y, p.Positions[0].Z)
	}
	for _, object := range sc.Objects {
		if object.StartingSlot == 0 {
			continue
		}
		c, r := bounds(object.Parts)
		state := ""
		if object.Dead {
			state = " [мёртвый]"
		}
		fmt.Printf("  слот %d %-62s центр=(%.1f, %.1f, %.1f) радиус=%.1f%s\n",
			object.StartingSlot, baseName(object.Name), c[0], c[1], c[2], r, state)
	}
	for _, skip := range sc.Skipped {
		fmt.Printf("  пропущен %s: %s\n", skip.Name, skip.Reason)
	}
}

func bounds(parts []sgbview.Part) ([3]float64, float64) {
	var min, max [3]float64
	first := true
	for _, part := range parts {
		if part.Mesh == nil {
			continue
		}
		for _, v := range part.Mesh.Vertices {
			p := [3]float64{float64(v.Pos[0]), float64(v.Pos[1]), float64(v.Pos[2])}
			if first {
				min, max = p, p
				first = false
				continue
			}
			for a := range p {
				min[a] = math.Min(min[a], p[a])
				max[a] = math.Max(max[a], p[a])
			}
		}
	}
	var c [3]float64
	for a := range c {
		c[a] = (min[a] + max[a]) * 0.5
	}
	dx, dy, dz := max[0]-min[0], max[1]-min[1], max[2]-min[2]
	return c, math.Sqrt(dx*dx+dy*dy+dz*dz) * 0.5
}

func baseName(path string) string {
	p := path
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			p = p[i+1:]
			break
		}
	}
	return p
}

package attrib

import (
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// Weapon — дальнобойное оружие бойца: дистанция, точность, темп стрельбы и
// урон по классам брони. Механика боя берёт отсюда только эти числа (см.
// entities/combat.go), остальное оружия — эффекты, звук, снаряды — пока не
// читается.
//
// У бойца в combat_ext несколько хардпоинтов, и обычно один из них —
// стрелковый, другой — ближнего боя (у некрон-воина
// necron_warrior_gauss_flayer_ranged и _melee). Читается первый стрелковый:
// ближний бой пока не симулируется.
type Weapon struct {
	// Path — путь чертежа оружия (Data/attrib/weapon/...rgd).
	Path string
	// Name — имя чертежа без пути и расширения.
	Name string
	// MinRange, MaxRange — дистанция стрельбы в метрах мира. В чертеже она
	// в условных футах движка, как и скорость, поэтому переводится тем же
	// коэффициентом speedToWorld.
	MinRange, MaxRange float64
	// Accuracy — доля попаданий: 0.8 — 80 % выстрелов.
	Accuracy float64
	// ReloadTime — секунды между выстрелами.
	ReloadTime float64
	// MinDamage, MaxDamage — разброс урона за попадание.
	MinDamage, MaxDamage float64
	// Melee — оружие ближнего боя (melee_weapon): в стрельбе не участвует.
	Melee bool
	// Piercing — урон оружия по классам брони в процентах: ключ — тип брони
	// цели ("tp_infantry_heavy_med"), значение — armour_piercing_value.
	Piercing map[string]float64
}

// ReadWeapon читает чертёж оружия по ссылке из хардпоинта
// ("weapon\necron_warrior_gauss_flayer_ranged.lua").
func ReadWeapon(tree *modapi.ModTree, ref string) (Weapon, error) {
	path := refPath(ref)
	node, err := readBlueprint(tree, path)
	if err != nil {
		return Weapon{}, err
	}
	weapon := Weapon{Path: path, Name: refName(path)}
	weapon.Melee, _ = node.BoolAt("melee_weapon")
	weapon.Accuracy = float64(numOr(node, "accuracy"))
	weapon.ReloadTime = float64(numOr(node, "reload_time"))
	weapon.MinRange = float64(numOr(node, "min_range")) * speedToWorld
	weapon.MaxRange = float64(numOr(node, "max_range")) * speedToWorld
	damage := weaponDamage(node)
	weapon.MinDamage = float64(numOr(damage, "min_damage"))
	weapon.MaxDamage = float64(numOr(damage, "max_damage"))
	if weapon.MaxDamage < weapon.MinDamage {
		weapon.MaxDamage = weapon.MinDamage
	}
	weapon.Piercing = readPiercing(damage)
	return weapon, nil
}

// readRangedWeapon находит в combat_ext главное стрелковое оружие: идёт по
// хардпоинтам и их weapon_table, ближний бой пропускает и берёт самое
// дальнобойное. Порядок хардпоинтов в чертеже не задаёт главное орудие: у
// монолита первым стоит тяжёлый гаусс-каннон (9 м), а его основное орудие —
// particle whip (17 м), и по первому он бы стрелял вдвое ближе, чем может.
// nil — стрелять нечем (постройка, рабочая единица без хардпоинтов).
func readRangedWeapon(tree *modapi.ModTree, node *rgdmeta.Node) *Weapon {
	hardpoints := node.Get("combat_ext", "hardpoints")
	if hardpoints == nil {
		return nil
	}
	var best *Weapon
	for _, hardpoint := range hardpoints.Children {
		table := hardpoint.Get("weapon_table")
		if table == nil {
			continue
		}
		for _, slot := range table.Children {
			ref, ok := slot.StrAt("weapon")
			if !ok || strings.TrimSpace(ref) == "" {
				continue
			}
			weapon, err := ReadWeapon(tree, ref)
			if err != nil {
				continue
			}
			if weapon.Melee || weapon.MaxRange <= 0 {
				continue
			}
			if best == nil || weapon.MaxRange > best.MaxRange {
				copy := weapon
				best = &copy
			}
		}
	}
	return best
}

// weaponDamage — раздел с уроном: обычно area_effect/weapon_damage, но у
// оружия без зоны он лежит прямо в weapon_damage.
func weaponDamage(node *rgdmeta.Node) *rgdmeta.Node {
	if node == nil {
		return nil
	}
	if damage := node.Get("weapon_damage", "armour_damage"); damage != nil {
		return damage
	}
	return node.Get("area_effect", "weapon_damage", "armour_damage")
}

// readPiercing читает таблицу пробития: запись на каждый класс брони, внутри
// ссылка на тип брони и процент урона.
func readPiercing(damage *rgdmeta.Node) map[string]float64 {
	types := damage.Get("armour_piercing_types")
	if types == nil {
		return nil
	}
	out := make(map[string]float64, len(types.Children))
	for _, entry := range types.Children {
		if entry == nil {
			continue
		}
		class := ""
		if armourType := entry.Get("armour_type"); armourType != nil {
			class = refName(armourType.Ref)
			if class == "" {
				class = refName(armourType.Str)
			}
		}
		if class == "" {
			continue
		}
		value, ok := numAt(entry, "armour_piercing_value")
		if !ok {
			continue
		}
		out[class] = float64(value)
	}
	return out
}

// refName — последний элемент ссылки без расширения: "ebps\races\...\name.lua"
// → "name", "type_armour\tp_infantry_med.lua" → "tp_infantry_med".
func refName(ref string) string {
	name := strings.TrimSpace(ref)
	name = strings.ReplaceAll(name, `\`, "/")
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[:dot]
	}
	return name
}

// numOr — число по имени поля или нуль.
func numOr(node *rgdmeta.Node, name string) float32 {
	value, _ := numAt(node, name)
	return value
}

// Package attrib читает игровые чертежи Dawn of War из папки Data/attrib:
// юнитов и построек (ebps) и отрядов (sbps).
//
// В игре юнит и отряд — два разных чертежа. Постройка живёт только в ebps,
// а у юнита есть и то, и другое: ebps описывает одного бойца (здоровье,
// стоимость, иконка, модель), sbps — отряд целиком (сколько бойцов в нём
// по умолчанию и максимум, кого можно пополнять, кто умеет захватывать
// точки). Связь одна направлена: sbps указывает на ebps в таблицах
// squad_loadout_ext/trooper_base и squad_leader_ext/leader_NN/leader
// (путь вида "ebps\races\...\trooper.lua"); обратной ссылки в ebps нет.
//
// Разбор — общий (пакет rgdmeta): здесь только известные поля, нужные
// игровой логике и HUD.
package attrib

import (
	"fmt"
	"strings"

	"dow-mod-studio/modapi"

	"github.com/Anykey-Nomad/Open-Kaurava/rgdmeta"
)

// Cost — цена в валютах игры. Ненулевые поля — те, что чертёж вообще пишет;
// у большинства рас это requisition и power, но модули добавляют faith и souls.
type Cost struct {
	Requisition float64
	Power       float64
	Population  float64
	Faith       float64
	Souls       float64
}

// Empty — цены нет ни в одной валюте.
func (c Cost) Empty() bool {
	return c == Cost{}
}

// String печатает цену так, как её читает игрок: компоненты через запятую,
// пустая цена — «—».
func (c Cost) String() string {
	var parts []string
	add := func(label string, value float64) {
		if value != 0 {
			parts = append(parts, fmt.Sprintf("%s %g", label, value))
		}
	}
	add("очка", c.Requisition)
	add("энергии", c.Power)
	add("населения", c.Population)
	add("веры", c.Faith)
	add("душ", c.Souls)
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

// readBlueprint читает и разбирает чертёж по пути дерева модуля.
func readBlueprint(tree *modapi.ModTree, path string) (*rgdmeta.Node, error) {
	if tree == nil {
		return nil, fmt.Errorf("attrib: дерево модуля не готово")
	}
	data, err := tree.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("attrib: чтение %s: %w", path, err)
	}
	node, err := rgdmeta.Parse(data, nil)
	if err != nil {
		return nil, fmt.Errorf("attrib: разбор %s: %w", path, err)
	}
	return node, nil
} // cost читает стандартную пару «time_seconds + cost» (так устроены и цена
// юнита, и пополнение отряда). Валюты лежат во вложенной таблице cost,
// а у birth_resource — прямо в самой таблице.
func cost(node *rgdmeta.Node) (Cost, float32) {
	if node == nil {
		return Cost{}, 0
	}
	currencies := node.Get("cost")
	if currencies == nil {
		currencies = node
	}
	read := func(name string) float64 {
		if value, ok := currencies.FloatAt(name); ok {
			return float64(value)
		}
		if value, ok := currencies.IntAt(name); ok {
			return float64(value)
		}
		return 0
	}
	out := Cost{
		Requisition: read("requisition"),
		Power:       read("power"),
		Population:  read("population"),
		Faith:       read("faith"),
		Souls:       read("souls"),
	}
	// pop — так валюта называется в squad_loadout_ext/birth_resource.
	if out.Population == 0 {
		if value, ok := currencies.IntAt("pop"); ok {
			out.Population = float64(value)
		}
	}
	seconds := float32(0)
	if value, ok := node.FloatAt("time_seconds"); ok {
		seconds = value
	} else if value, ok := node.IntAt("time_seconds"); ok {
		seconds = float32(value)
	}
	return out, seconds
}

// refPath превращает ссылку из чертежа ("ebps\races\...\name.lua") в путь
// дерева модуля ("Data/attrib/ebps/.../name.rgd"). Игра пишет ссылки и с
// .lua, и без расширения, и слэши — обратные.
func refPath(ref string) string {
	p := strings.ReplaceAll(strings.TrimSpace(ref), `\`, "/")
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(p), "ebps/") && !strings.HasPrefix(strings.ToLower(p), "sbps/") {
		// Ссылка уже от корня Data/attrib или от корня дерева — оставляем как есть.
		if !strings.HasPrefix(strings.ToLower(p), "data/") {
			p = "Data/attrib/" + p
		}
	} else {
		p = "Data/attrib/" + p
	}
	lower := strings.ToLower(p)
	lastSlash := strings.LastIndexByte(p, '/')
	if dot := strings.LastIndexByte(lower, '.'); dot > lastSlash {
		p = p[:dot]
	}
	if !strings.HasSuffix(lower, ".rgd") {
		p += ".rgd"
	}
	return p
}

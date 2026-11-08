package scenes

import (
	"fmt"
	"github.com/Anykey-Nomad/Open-Kaurava/loc"
	"math"

	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// siteModelResult — модель постройки, прочитанная в фоне для заложенной
// площадки. В рендерер она попадает уже в главном потоке: загрузка в
// видеопамять — это команды GPU, их нельзя разносить по потокам.
type siteModelResult struct {
	siteID string
	object *sgbview.Object
	err    error
}

// requestSiteModel запускает фоновую загрузку модели постройки. Пока модели
// нет, площадка рисуется коробкой, а после загрузки — самой моделью с
// анимацией стройки.
func (p *Gameplay) requestSiteModel(siteID, blueprint string) {
	if p.lib == nil || blueprint == "" || p.hud == nil {
		return
	}
	site := p.sites[siteID]
	if site == nil {
		return
	}
	tree := p.lib.Tree()
	if tree == nil {
		return
	}
	if p.siteModels == nil {
		p.siteModels = make(chan siteModelResult, 16)
	}
	channel := p.siteModels
	at, ground := site.center, site.ground
	// Имя меша уникально для площадки: по нему рендерер находит объект,
	// когда приходят кадры стройки и когда работа закончена.
	name := "site-" + siteID
	go func() {
		object, err := sgbview.ObjectForBlueprint(tree, blueprint, at[0], at[1], ground, 0, name, false)
		channel <- siteModelResult{siteID: siteID, object: object, err: err}
	}()
}

// updateSiteModels принимает готовые модели площадок: кладёт их в сцену и
// заводит объект карты — по нему работает выбор лучом и рисуется индикатор
// выделения из DATASEUI (круг или квадрат).
func (p *Gameplay) updateSiteModels() {
	if p.siteModels == nil {
		return
	}
	for {
		select {
		case result := <-p.siteModels:
			site := p.sites[result.siteID]
			if site == nil {
				// Площадка исчезла, пока модель читалась: результат не нужен.
				continue
			}
			if result.object == nil || p.world == nil {
				if p.hud != nil {
					p.hud.command = loc.T("Building model not loaded: ") + site.label
				}
				continue
			}
			if err := p.world.AddObject(*result.object, false); err != nil {
				if p.hud != nil {
					p.hud.command = loc.T("Building model not loaded: ") + site.label
				}
				continue
			}
			site.meshName = result.object.MeshName()
			p.addSiteObject(site, result.object)
		default:
			return
		}
	}
}

// unitModelResult — модель бойца, прочитанная в фоне после найма.
type unitModelResult struct {
	entityID string
	object   *sgbview.Object
	err      error
}

// requestUnitModel запускает фоновую загрузку модели нанятого бойца: пока
// модели нет, юнит числится на карте, но не виден и не выбирается лучом.
func (p *Gameplay) requestUnitModel(entityID, blueprint string, x, y, ground float64) {
	if p.lib == nil || blueprint == "" || p.hud == nil {
		return
	}
	tree := p.lib.Tree()
	if tree == nil {
		return
	}
	if p.unitModels == nil {
		p.unitModels = make(chan unitModelResult, 32)
	}
	channel := p.unitModels
	// Имя меша уникально для каждой загрузки: поднятое тело получает новую
	// модель, а скрытый труп со старым именем остаётся в рендерере.
	p.modelSerial++
	name := fmt.Sprintf("unit-%s-%d", entityID, p.modelSerial)
	go func() {
		// Нанятый боец — с подъёмом: иначе модель уходит по пояс в землю.
		object, err := sgbview.ObjectForBlueprint(tree, blueprint, x, y, ground, 0, name, true)
		channel <- unitModelResult{entityID: entityID, object: object, err: err}
	}()
}

// updateUnitModels принимает готовые модели бойцов: кладёт их в сцену и
// заводит объекты карты — с этого момента юнит виден, выбирается и ходит.
func (p *Gameplay) updateUnitModels() {
	if p.unitModels == nil {
		return
	}
	for {
		select {
		case result := <-p.unitModels:
			entity, ok := p.hud.factory.Get(result.entityID)
			if !ok || result.object == nil || p.world == nil {
				continue
			}
			if err := p.world.AddObject(*result.object, true); err != nil {
				continue
			}
			center, radius := objectBounds(*result.object)
			// Поднятое тело уже есть на карте своим трупом: новый меш
			// занимает его место, а не появляется вторым объектом.
			if replaced := p.replaceUnitModel(entity, result.object, center, radius); replaced {
				continue
			}
			p.objects = append(p.objects, gameplayObject{
				id: entity.ID, name: entity.Name, owner: entity.Owner, kind: entities.KindUnit,
				selectable: entity.Selectable,
				selected:   result.object.Selected,
				center:     center,
				radius:     math.Max(radius, 1.5),
				zOffset:    center[2] - p.groundHeight(center[0], center[1]),
				parts:      result.object.Parts,
				meshName:   result.object.MeshName(),
			})
		default:
			return
		}
	}
}

// replaceUnitModel переносит готовую модель на место уже существующего
// объекта с тем же ID (поднятое тело). Старый меш к этому моменту скрыт, а
// объект ждёт новый — c пустым именем меша. false — объекта с таким ID нет.
func (p *Gameplay) replaceUnitModel(entity entities.Entity, object *sgbview.Object, center [3]float64, radius float64) bool {
	for i := range p.objects {
		o := &p.objects[i]
		if o.id != entity.ID {
			continue
		}
		o.name = entity.Name
		o.owner = entity.Owner
		o.kind = entities.KindUnit
		o.selectable = entity.Selectable
		o.selected = object.Selected
		o.center = center
		o.radius = math.Max(radius, 1.5)
		o.zOffset = center[2] - p.groundHeight(center[0], center[1])
		o.parts = object.Parts
		o.meshName = object.MeshName()
		o.moveOffset = [3]float64{}
		o.deathAge = 0
		o.hidden = false
		return true
	}
	return false
}

// addSiteObject заводит объект карты для заложенной площадки: с него берутся
// выбор лучом, индикатор выделения и данные панели свойств.
func (p *Gameplay) addSiteObject(site *constructionSite, object *sgbview.Object) {
	center, radius := objectBounds(*object)
	owner := p.hud.ownPlayer
	if entity, ok := p.hud.factory.Get(site.buildingID); ok {
		owner = entity.Owner
	}
	p.objects = append(p.objects, gameplayObject{
		id: site.buildingID, name: site.label, owner: owner, kind: entities.KindBuilding,
		selectable: true,
		selected:   object.Selected,
		center:     center,
		radius:     math.Max(radius, 2),
		zOffset:    center[2] - p.groundHeight(center[0], center[1]),
		parts:      object.Parts,
		meshName:   site.meshName,
	})
}

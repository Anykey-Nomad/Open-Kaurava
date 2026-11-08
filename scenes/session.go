package scenes

import (
	"time"

	"dow-mod-studio/sgbview"
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/fx"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
)

// Сессия игры: выход в главное меню обязан её сбросить, иначе следующее
// «Начать» продолжало бы прошлую партию — фабрика объектов, площадки,
// очереди найма и выделение живут в сцене, а сцена создаётся одна на
// приложение (main.go). Здесь же сброс перед перезапуском сценария.

// resetSession возвращает сцену в состояние «до загрузки»: следующий вход в
// геймплей начинает выбранный сценарий заново.
func (p *Gameplay) resetSession() {
	if p == nil {
		return
	}
	p.stopMusic()
	p.pause.closeState()
	if p.world != nil {
		p.world.ClearScene()
	}
	if p.fx != nil {
		p.fx.Clear()
	}
	p.want, p.loaded, p.builds = "", "", ""
	p.pending, p.loading, p.started = nil, false, time.Time{}
	p.err, p.info, p.warn = nil, "", ""
	p.heights, p.heightXs, p.heightYs = nil, nil, nil
	p.objects = nil
	p.sites = map[string]*constructionSite{}
	p.place = placementState{}
	p.siteModels, p.unitModels = nil, nil
	p.modelSerial = 0
	p.queues = map[string]*trainQueue{}
	p.squads = map[string][]string{}
	p.entitySquad = map[string]string{}
	p.nextSquad = 0
	p.harvest = map[string]*harvestState{}
	p.cooldowns = map[string]float64{}
	p.capture = map[string]*captureVisual{}
	p.reinforce = map[string]*reinforceTask{}
	p.fxShots = map[string]uint64{}
	p.fxDead = map[string]bool{}
	p.shotFX = nil
	p.shotDefs = map[string][]sgbview.ShotFX{}
	p.fxFiles = map[string]*fx.File{}
	p.fxClock = 0
	p.pass = nil
	if p.hud != nil {
		// Фабрика — вся экономика и все объекты боя: новая фабрика значит
		// пустую карту до следующей загрузки сценария. Игроки и постройки
		// регистрируются заново при сборке сцены (object_pick.go).
		p.hud.factory = entities.NewFactory()
		p.hud.raceCaps = nil
		p.hud.selectedID, p.hud.selectedIDs, p.hud.multiCards = "", nil, nil
		p.hud.command = ""
		p.hud.placeBlueprint, p.hud.placeLabel = "", ""
		p.hud.orderMode, p.hud.pendingOrder, p.hud.orderPage = orderModeRoot, orderNone, 0
		p.hud.paused = false
	}
}

// leaveToMainMenu сбрасывает сессию и выходит в главное меню: музыка боя
// замолкает, карта и объекты освобождаются.
func (p *Gameplay) leaveToMainMenu(c *scene.Ctx) {
	p.resetSession()
	if c != nil && c.Switch != nil {
		c.Switch("menu")
	}
}

// restartScenario перезапускает выбранный сценарий: сессия сбрасывается, и
// ближайший кадр загружает карту заново (см. updateLoad).
func (p *Gameplay) restartScenario(c *scene.Ctx) {
	p.resetSession()
	if c == nil || c.Switch == nil {
		return
	}
}

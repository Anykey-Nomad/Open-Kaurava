package scenes

import (
	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/scene"
	"github.com/Anykey-Nomad/Open-Kaurava/sfx"
)

// Звуки игрового процесса: щелчки интерфейса и отклики приказов.
// Источники и разрешение озвучки — в пакете sfx.

func init() {
	// Каждый клик ЛКМ по кнопке меню или HUD звучит (один раз за кадр —
	// см. scene.Ctx.ClickedAt). В тестах файл встроен, поэтому звук
	// либо проигрывается, либо тихо пропускается.
	scene.OnUIClick = sfx.UIClick
}

// playOrderSound — голос юнита на принятый приказ. Голос разрешается по
// озвучке чертежа (ui_ext/speech_directory): папка юнита в озвучке мода и
// категория действия — «attack» для атаки, «capture» для захвата, «move»
// для движения. Если у юнита своей реплики нет — звучит общий сигнал
// приказа.
func (p *Gameplay) playOrderSound(selected entities.Entity, order entities.Order) {
	if p.hud == nil || p.hud.library == nil {
		return
	}
	speech := p.hud.speechOf(selected)
	tree := p.hud.library.Tree()
	switch {
	case order.Attack:
		sfx.AttackAck(tree, speech)
	case order.Capture:
		sfx.OrderAck(tree, speech, true)
	default:
		sfx.OrderAck(tree, speech, false)
	}
}

// speechOf — папка озвучки сущности из её чертежа (ui_ext/speech_directory).
// Для нанятых в игре бойцов чертёж дочитывается по пути из sbps, поэтому
// голос находится и у них, а не только у объектов сценария.
func (g *GameplayHUD) speechOf(selected entities.Entity) string {
	if g == nil || g.attribData == nil {
		return ""
	}
	if unit, ok := g.attribData.unitFor(g.blueprintOf(selected)); ok {
		return unit.Speech
	}
	return ""
}

// selectVoice — голос выбранного отряда («выбран»). Без сигнала-заглушки.
func (g *GameplayHUD) selectVoice() {
	if g == nil || g.library == nil {
		return
	}
	selected, ok := g.selectedEntity()
	if !ok {
		return
	}
	sfx.SelectVoice(g.library.Tree(), g.speechOf(selected))
}

package scenes

import (
	"math"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// FX сцены: вспышки выстрелов, искры попаданий, дым повреждённых построек и
// пыль стройки. Частицы живут в fx.System, рисует их gfx3d, а сцена решает,
// где и когда их выпустить. События берутся из состояния сущностей, а не из
// внутренностей боя: у бойца растёт счётчик выстрелов, у постройки — прогресс
// стройки и здоровье.

// fxSmokeInterval — как часто проверять постройки на дым и пыль: каждый
// кадр клуб дыма не нужен, раз в треть секунды — достаточно.
const fxSmokeInterval = 0.3

// updateFX обновляет частицы и рождает новые по событиям мира. На паузе
// частицы замирают: dt обнуляется, кадр не двигает эффекты.
func (p *Gameplay) updateFX(dt float64) {
	if p.fx == nil || p.hud == nil || p.hud.factory == nil {
		return
	}
	if p.hud.paused {
		dt = 0
	}
	p.fx.Update(dt)
	p.stepShotFX(dt)
	if p.fxShots == nil {
		p.fxShots = map[string]uint64{}
	}
	p.fxClock += dt
	smoke := p.fxClock >= fxSmokeInterval
	if smoke {
		p.fxClock = 0
	}
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" {
			continue
		}
		entity, ok := p.hud.factory.Get(o.id)
		if !ok {
			continue
		}
		// Вспышка на каждый выстрел: счётчик выстрелов у бойца вырос.
		if shots := entity.Shots; shots != p.fxShots[o.id] {
			p.fxShots[o.id] = shots
			p.spawnShotFX(entity, *o)
		}
		if !smoke || !entity.Alive {
			continue
		}
		ground := [3]float64{o.center[0], o.center[1], o.center[2]}
		if entity.UnderConstruction() {
			p.fx.SpawnDust(ground, math.Max(o.radius*0.6, 1))
			continue
		}
		// Дым идёт от повреждённой постройки: ниже 60 % здоровья она
		// начинает «чадить», как в оригинале.
		if entity.Kind == entities.KindBuilding && entity.Health.Max > 0 &&
			entity.Health.Current/entity.Health.Max < 0.6 {
			top := [3]float64{ground[0], ground[1], ground[2] + math.Max(o.radius*0.4, 1)}
			p.fx.SpawnSmoke(top, math.Max(o.radius*0.5, 1.5))
		}
	}
}

// spawnShotFX рисует один выстрел: настоящий эффект из DATA:ART/FX у ствола
// и искры в точке попадания. Вызывается только когда счётчик выстрелов вырос.
//
// Процедурная вспышка остаётся запасным вариантом: у модели может не быть
// эффектов стрельбы в .whe. Искры — только при попадании: промах эффект
// проводит мимо цели, и искр в ней быть не должно.
func (p *Gameplay) spawnShotFX(shooter entities.Entity, object gameplayObject) {
	if p.fx == nil {
		return
	}
	real := p.spawnWeaponFX(shooter, object)
	muzzle := [3]float64{
		object.center[0] + math.Cos(shooter.Facing),
		object.center[1] + math.Sin(shooter.Facing),
		object.center[2] + 1.2,
	}
	if !real {
		p.fx.SpawnFlash(muzzle)
	}
	if !shooter.LastShotHit || shooter.LastShotTarget == "" || p.hud.factory == nil {
		return
	}
	target, ok := p.hud.factory.Get(shooter.LastShotTarget)
	if !ok {
		return
	}
	hit := [3]float64{target.Position.X, target.Position.Y, object.center[2] + 1}
	if targetObject, ok := p.objectByID(target.ID); ok {
		hit[2] = targetObject.center[2] + math.Max(targetObject.radius*0.4, 0.5)
	}
	p.fx.SpawnSparks(hit, 3)
}

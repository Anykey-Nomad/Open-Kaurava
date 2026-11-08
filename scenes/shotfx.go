package scenes

import (
	"math"
	"strings"

	"dow-mod-studio/modapi"
	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
	"github.com/Anykey-Nomad/Open-Kaurava/fx"
)

// Эффекты выстрелов из настоящих файлов DATA:ART/FX.
//
// На каждый выстрел сцена запускает свою симуляцию эффекта, привязанную к
// маркеру ствола. Событие стрельбы из .whe говорит, где вспышка (marker) и
// куда она идёт: fx_other=target — второй конец на том, в кого стреляли, а с
// fx_length=distance лента тянется ровно до него. Промах по accuracy
// смещает конец мимо цели, и эффект проходит рядом, а не заканчивается на
// ней.

// maxShotFX — предел одновременных эффектов выстрелов: залп отряда не
// должен утопить кадр в частицах.
const maxShotFX = 64

// activeShotFX — один играющий эффект выстрела.
type activeShotFX struct {
	sim *fx.Simulation
	// age и life — сколько эффект уже играет и сколько ему отведено: один
	// проход клипа, а не вечный цикл.
	age, life float64
	// origin — мировая точка ствола; yaw — поворот эффекта вокруг вертикали.
	origin [3]float64
	yaw    float64
	// end — второй конец эффекта в мире, hasEnd — он вообще есть.
	end    [3]float64
	hasEnd bool
	// spans — Length ленты означает долю зазора между концами.
	spans bool
}

// spawnWeaponFX запускает эффекты выстрела из .whe. true — настоящий эффект
// сыграл, и процедурную вспышку рисовать не надо.
func (p *Gameplay) spawnWeaponFX(shooter entities.Entity, object gameplayObject) bool {
	if p.world == nil || object.meshName == "" {
		return false
	}
	if p.shotDefs == nil {
		p.shotDefs = map[string][]sgbview.ShotFX{}
	}
	defs, ok := p.shotDefs[object.meshName]
	if !ok {
		defs = p.world.ShotEffects(object.meshName)
		p.shotDefs[object.meshName] = defs
	}
	if len(defs) == 0 {
		return false
	}
	target, hasTarget := p.hud.factory.Get(shooter.LastShotTarget)
	spawned := false
	for _, def := range defs {
		file := p.loadShotFXFile(def.Effect)
		if file == nil {
			continue
		}
		sim := fx.NewSimulation(file)
		origin, found := p.world.MarkerPoint(object.meshName, def.Marker)
		if !found {
			// Маркера нет — вспышка всё равно нужна: дуло там, где
			// боец смотрит, на высоте корпуса.
			origin = [3]float64{
				object.center[0] + math.Cos(shooter.Facing)*1.2,
				object.center[1] + math.Sin(shooter.Facing)*1.2,
				object.center[2] + object.zOffset*0.5,
			}
		}
		effect := activeShotFX{
			sim:    sim,
			life:   sim.Lifetime(),
			origin: origin,
			// Эффект авторится повёрнутым вперёд по -Y; +π/2 ставит его
			// вдоль +X мира, куда смотрит боец при facing = 0.
			yaw:   shooter.Facing + sgbview.ModelFacingOffset,
			spans: def.SpansDistance,
		}
		if def.OtherMarker != "" {
			if point, ok := p.world.MarkerPoint(object.meshName, def.OtherMarker); ok {
				effect.end, effect.hasEnd = point, true
			}
		}
		if strings.EqualFold(def.Other, "target") && hasTarget {
			effect.end = p.shotEndPoint(shooter, target, shooter.LastShotHit)
			effect.hasEnd = true
		}
		p.shotFX = append(p.shotFX, effect)
		if len(p.shotFX) > maxShotFX {
			p.shotFX = p.shotFX[len(p.shotFX)-maxShotFX:]
		}
		spawned = true
	}
	return spawned
}

// shotEndPoint — куда приходит выстрел. Попал — в центр цели; промахнулся —
// рядом с ней: конец сдвигается вбок на радиус цели и чуть дальше, чтобы
// эффект прошёл мимо, а не оборвался в воздухе.
func (p *Gameplay) shotEndPoint(shooter, target entities.Entity, hit bool) [3]float64 {
	end := [3]float64{target.Position.X, target.Position.Y, 1}
	if targetObject, ok := p.objectByID(target.ID); ok {
		end = [3]float64{targetObject.center[0], targetObject.center[1], targetObject.center[2]}
	}
	if hit {
		return end
	}
	dx, dy := end[0]-shooter.Position.X, end[1]-shooter.Position.Y
	length := math.Hypot(dx, dy)
	if length < 1e-6 {
		return end
	}
	dirX, dirY := dx/length, dy/length
	radius := 1.5
	if targetObject, ok := p.objectByID(target.ID); ok {
		radius = math.Max(targetObject.radius, 1)
	}
	offset := radius + 1.5
	// Вбок и немного дальше: видно, что выстрел прошёл рядом с целью.
	end[0] += -dirY*offset + dirX*0.5
	end[1] += dirX*offset + dirY*0.5
	return end
}

// stepShotFX двигает эффекты выстрелов и убирает отыгравшие.
func (p *Gameplay) stepShotFX(dt float64) {
	if dt <= 0 || len(p.shotFX) == 0 {
		return
	}
	kept := p.shotFX[:0]
	for i := range p.shotFX {
		shot := p.shotFX[i]
		shot.age += dt
		if shot.age >= shot.life {
			continue
		}
		shot.sim.Step(dt)
		kept = append(kept, shot)
	}
	p.shotFX = kept
}

// shotEffectFrame собирает частицы и ленты всех эффектов выстрелов в мире.
// eye — мировая точка камеры: по ней ленты поворачивают сечение.
func (p *Gameplay) shotEffectFrame(eye [3]float64) ([]fx.Particle, []fx.Quad) {
	if len(p.shotFX) == 0 {
		return nil, nil
	}
	var particles []fx.Particle
	var quads []fx.Quad
	for i := range p.shotFX {
		shot := &p.shotFX[i]
		for _, particle := range shot.sim.Frame() {
			particles = append(particles, shot.particleToWorld(particle))
		}
		from := fx.Vec3{}
		end := fx.Vec3{X: 1}
		if shot.hasEnd {
			end = shot.worldToFile(shot.end)
		}
		eyeLocal := shot.worldToFile(eye)
		for _, quad := range shot.sim.QuadsAt(eyeLocal, from, end, shot.hasEnd && shot.spans) {
			for corner := range quad.P {
				quad.P[corner] = shot.fileToWorld(quad.P[corner])
			}
			quads = append(quads, quad)
		}
	}
	return particles, quads
}

// particleToWorld переносит спрайт эффекта из его системы в мир: Frame уже
// перевёл оси файла в оси мира, остаётся поворот и точка ствола.
func (s *activeShotFX) particleToWorld(particle fx.Particle) fx.Particle {
	x, y := rotateFX(particle.Pos[0], particle.Pos[1], s.yaw)
	particle.Pos = [3]float64{s.origin[0] + x, s.origin[1] + y, s.origin[2] + particle.Pos[2]}
	return particle
}

// fileToWorld переводит точку из системы файла (Y вверх, Z вперёд) в мир.
func (s *activeShotFX) fileToWorld(v fx.Vec3) fx.Vec3 {
	x, y := rotateFX(v.X, -v.Z, s.yaw)
	return fx.Vec3{X: s.origin[0] + x, Y: s.origin[1] + y, Z: s.origin[2] + v.Y}
}

// worldToFile — обратное преобразование: мировая точка в систему эффекта.
func (s *activeShotFX) worldToFile(point [3]float64) fx.Vec3 {
	dx, dy, dz := point[0]-s.origin[0], point[1]-s.origin[1], point[2]-s.origin[2]
	cos, sin := math.Cos(-s.yaw), math.Sin(-s.yaw)
	x := dx*cos - dy*sin
	y := dx*sin + dy*cos
	return fx.Vec3{X: x, Y: dz, Z: -y}
}

// rotateFX поворачивает горизонтальную пару на угол.
func rotateFX(x, y, angle float64) (float64, float64) {
	cos, sin := math.Cos(angle), math.Sin(angle)
	return x*cos - y*sin, x*sin + y*cos
}

// loadShotFXFile читает и разбирает файл эффекта из Data/art/fx. Игра пишет
// путь без расширения и с обратными слэшами; файл ищется и по полному пути,
// и по имени — как текстуры эффектов.
func (p *Gameplay) loadShotFXFile(name string) *fx.File {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return nil
	}
	if p.fxFiles == nil {
		p.fxFiles = map[string]*fx.File{}
	}
	if file, ok := p.fxFiles[key]; ok {
		return file
	}
	tree := p.effectTree()
	if tree == nil {
		return nil
	}
	p.fxFiles[key] = nil // неудачный разбор не повторяем каждый выстрел
	rel := strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	for _, prefix := range []string{"data:", "data/", "./"} {
		if len(rel) >= len(prefix) && strings.EqualFold(rel[:len(prefix)], prefix) {
			rel = rel[len(prefix):]
		}
	}
	rel = strings.TrimLeft(rel, "/")
	if !strings.HasPrefix(strings.ToLower(rel), "art/fx/") {
		rel = "art/fx/" + rel
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".lua") {
		rel += ".lua"
	}
	for _, candidate := range []string{"Data/" + rel, rel, strings.ToLower("Data/" + rel), strings.ToLower(rel)} {
		data, err := tree.ReadFile(candidate)
		if err != nil || len(data) == 0 {
			continue
		}
		file, err := fx.ParseFile(data)
		if err != nil || file == nil {
			return nil
		}
		p.fxFiles[key] = file
		return file
	}
	return nil
}

// effectTree — дерево модуля, по которому читаются файлы эффектов.
func (p *Gameplay) effectTree() *modapi.ModTree {
	if p.hud != nil && p.hud.attribData != nil && p.hud.attribData.tree != nil {
		return p.hud.attribData.tree
	}
	if p.lib != nil {
		return p.lib.Tree()
	}
	return nil
}

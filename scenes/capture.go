package scenes

import (
	"strconv"
	"strings"

	"dow-mod-studio/sgbview"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

// Анимация точек захвата: стратегических позиций, целей и реликтов.
//
// В модели точки есть готовые клипы (у реликтов — с приставкой ol_):
//
//	uncaptured_open — стойка складывается (по умолчанию виден только её
//	                  первый кадр);
//	flag_plant_idle — флагшток поставлен, но флага нет;
//	captured_raise  — флаг поднимается: первый кадр 0 %, последний 100 %;
//	necron_raise    — то же самое для некронов (у них вместо флага эффект);
//	vis_<раса>_flag — видимость флага этой расы.
//
// Прогресс захвата хранит сама точка (entities.stepCapture), поэтому клип
// подъёма всегда стоит на его проценте: прерванный захват отыгрывает клип
// назад, а не пропадает рывком.
const (
	// captureIdle — никто не берёт точку: на экране первый кадр
	// uncaptured_open.
	captureIdle = iota
	// captureOpening — кто-то получил приказ захвата: uncaptured_open
	// играет до последнего кадра.
	captureOpening
	// capturePlanted — стойка раскрыта, флаг ещё не поднимается:
	// flag_plant_idle.
	capturePlanted
	// captureRaising — идёт захват (или снятие флага): captured_raise
	// стоит на проценте захвата.
	captureRaising
)

// captureVisual — что сейчас играет точка захвата: фаза, время клипа и
// примятая видимость частей, чтобы не трогать рендерер каждый кадр.
type captureVisual struct {
	phase     int
	time      float64
	partsKey  string
	framesKey string
}

// captureFlagSuffix — папка расы и приставка клипа её флага в модели точки.
// Папки — из Data/attrib/ebps/races, клипы — vis_<приставка>_flag.
var captureFlagSuffix = map[string]string{
	"space_marines": "marine",
	"guard":         "guard",
	"chaos":         "chaos",
	"eldar":         "eldar",
	"dark_eldar":    "dark_eldar",
	"orks":          "ork",
	"sisters":       "sisters",
	"tau":           "tau",
	"necrons":       "necron",
}

// playerSlot — номер слота игрока из его ID ("player-1" → 1); 0 — не слот.
func playerSlot(id entities.PlayerID) int {
	text := strings.TrimSpace(string(id))
	if !strings.HasPrefix(text, "player-") {
		return 0
	}
	slot, err := strconv.Atoi(strings.TrimPrefix(text, "player-"))
	if err != nil {
		return 0
	}
	return slot
}

// captureFlagClip — клип видимости флага расы игрока (vis_necron_flag и
// т. п.). Пусто — у игрока нет слота или расы.
func captureFlagClip(player entities.PlayerID) string {
	slot := playerSlot(player)
	if slot <= 0 {
		return ""
	}
	suffix, ok := captureFlagSuffix[raceFolderOf(slot)]
	if !ok {
		return ""
	}
	return "vis_" + suffix + "_flag"
}

// capturePlayerRace — папка расы игрока ("necrons"); пусто — не слот.
func capturePlayerRace(player entities.PlayerID) string {
	slot := playerSlot(player)
	if slot <= 0 {
		return ""
	}
	return raceFolderOf(slot)
}

// captureClip — как называется клип у модели объекта: у стратегических
// точек он без приставки, у реликвий — с ol_.
func (p *Gameplay) captureClip(mesh, base string) string {
	if p.world == nil || mesh == "" || base == "" {
		return ""
	}
	if p.world.HasObjectClip(mesh, base) {
		return base
	}
	if p.world.HasObjectClip(mesh, "ol_"+base) {
		return "ol_" + base
	}
	return ""
}

// updateCapturePoints ведёт анимацию всех точек захвата: кто получил приказ,
// на каком проценте стоит флаг и чья раса его держит.
func (p *Gameplay) updateCapturePoints(dt float64) {
	if p.hud == nil || p.hud.factory == nil || p.world == nil {
		return
	}
	if p.hud.paused {
		dt = 0
	}
	// Кто-то (свой или чужой) получил приказ захвата этой точки: стойка
	// раскрывается, как только приказ отдан, а не по прибытии.
	ordered := make(map[string]bool, len(p.objects))
	for i := range p.objects {
		entity, ok := p.hud.factory.Get(p.objects[i].id)
		if !ok || !entity.Alive || entity.Order == nil || !entity.Order.Capture {
			continue
		}
		if entity.OrderStatus != entities.OrderInProgress || entity.Order.TargetID == "" {
			continue
		}
		ordered[entity.Order.TargetID] = true
	}
	for i := range p.objects {
		o := &p.objects[i]
		if o.id == "" || o.meshName == "" {
			continue
		}
		point, ok := p.hud.factory.Get(o.id)
		if !ok || !point.Capturable {
			continue
		}
		p.animateCapturePoint(o, point, ordered[o.id], dt)
	}
}

// animateCapturePoint раскладывает состояние точки по фазам и отдаёт
// рендереру клип с кадром и видимость частей (флаг только у захватившей
// расы).
func (p *Gameplay) animateCapturePoint(o *gameplayObject, point entities.Entity, ordered bool, dt float64) {
	visual := p.capture[o.id]
	if visual == nil {
		visual = &captureVisual{}
		if p.capture == nil {
			p.capture = map[string]*captureVisual{}
		}
		p.capture[o.id] = visual
	}
	open := p.captureClip(o.meshName, "uncaptured_open")
	planted := p.captureClip(o.meshName, "flag_plant_idle")
	raise := p.captureClip(o.meshName, "captured_raise")
	necron := p.captureClip(o.meshName, "necron_raise")

	owned := point.Owner != "" && point.Owner != entities.NeutralPlayer
	progress := point.CaptureProgress
	// Флаг держит владелец точки; на нейтральной — тот, кто его поднимает
	// (у прерванного захвата флаг так и остаётся приподнятым, пока
	// прогресс не кончится).
	flagPlayer := point.Owner
	if !owned {
		flagPlayer = point.CapturePlayer
	}
	flagClip := captureFlagClip(flagPlayer)
	necronFlag := capturePlayerRace(flagPlayer) == "necrons"

	switch {
	case owned || progress > 0:
		// Захват идёт: высота флага — это и есть процент захвата.
		visual.phase = captureRaising
		frames := make([]sgbview.ClipFrame, 0, 2)
		if raise != "" {
			frames = append(frames, sgbview.ClipFrame{Name: raise, Progress: progress})
		}
		if necronFlag && necron != "" {
			frames = append(frames, sgbview.ClipFrame{Name: necron, Progress: progress})
		}
		p.driveCaptureClip(o, visual, frames)
		// Остов должен быть виден всегда: набор клипов для видимости берём с
		// клипом, который его описывает, даже если клипа подъёма нет.
		base := raise
		if base == "" {
			base = planted
		}
		if base == "" {
			base = open
		}
		clips := make([]string, 0, 3)
		if base != "" {
			clips = append(clips, base)
		}
		if flagClip != "" {
			clips = append(clips, flagClip)
		}
		if necronFlag && necron != "" {
			// Клип некронов показывает их эффект вместо флага: другим расам
			// он не нужен — его треки видимости включили бы некронские меши.
			clips = append(clips, necron)
		}
		p.showCaptureParts(o, visual, clips)
	case ordered:
		// Приказ отдан: стойка раскрывается до конца и остаётся раскрытой,
		// пока точку кто-то берёт.
		if visual.phase != captureOpening && visual.phase != capturePlanted {
			visual.phase = captureOpening
			visual.time = 0
		}
		if visual.phase == captureOpening {
			duration := p.world.ObjectClipDuration(o.meshName, open)
			visual.time += dt
			if duration <= 0 || visual.time >= duration {
				visual.phase = capturePlanted
				visual.time = 0
			} else {
				p.driveCaptureClip(o, visual, []sgbview.ClipFrame{{Name: open, Progress: visual.time / duration}})
				p.showCaptureParts(o, visual, []string{open})
				return
			}
		}
		duration := p.world.ObjectClipDuration(o.meshName, planted)
		visual.time += dt
		t := 0.0
		if duration > 0 {
			t = visual.time / duration
			t -= float64(int(t))
		}
		p.driveCaptureClip(o, visual, []sgbview.ClipFrame{{Name: planted, Progress: t}})
		p.showCaptureParts(o, visual, []string{planted})
	default:
		// Флагшток уже раскрывали — он остаётся стоять, а не складывается
		// назад: опустился только флаг.
		if visual.phase == captureOpening || visual.phase == capturePlanted || visual.phase == captureRaising {
			visual.phase = capturePlanted
			visual.time = 0
			p.driveCaptureClip(o, visual, []sgbview.ClipFrame{{Name: planted, Progress: 0}})
			p.showCaptureParts(o, visual, []string{planted})
			return
		}
		// Нетронутая точка: виден первый кадр uncaptured_open, а не закрытое
		// состояние из idle-клипа модели.
		visual.phase = captureIdle
		visual.time = 0
		p.driveCaptureClip(o, visual, []sgbview.ClipFrame{{Name: open, Progress: 0}})
		p.showCaptureParts(o, visual, []string{open})
	}
}

// driveCaptureClip отдаёт рендереру кадры клипов точки, если они изменились:
// подпись кадров считается по именам и процентам.
func (p *Gameplay) driveCaptureClip(o *gameplayObject, visual *captureVisual, frames []sgbview.ClipFrame) {
	if len(frames) == 0 {
		return
	}
	key := captureFramesKey(frames)
	if key == visual.framesKey {
		return
	}
	if p.world.SetObjectFrames(o.meshName, frames) {
		visual.framesKey = key
	}
}

// showCaptureParts включает части, видимые в наборе клипов: у точки это
// остов плюс флаг одной расы.
func (p *Gameplay) showCaptureParts(o *gameplayObject, visual *captureVisual, clips []string) {
	names := make([]string, 0, len(clips))
	for _, clip := range clips {
		if clip != "" {
			names = append(names, clip)
		}
	}
	if len(names) == 0 {
		return
	}
	key := strings.Join(names, "|")
	if key == visual.partsKey {
		return
	}
	if p.world.ShowObjectParts(o.meshName, names) {
		visual.partsKey = key
	}
}

// captureFramesKey — подпись набора кадров: имена клипов и проценты.
func captureFramesKey(frames []sgbview.ClipFrame) string {
	var builder strings.Builder
	for _, frame := range frames {
		builder.WriteString(frame.Name)
		builder.WriteByte(':')
		builder.WriteString(strconv.FormatFloat(frame.Progress, 'f', 4, 64))
		builder.WriteByte(';')
	}
	return builder.String()
}

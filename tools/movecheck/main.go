package main

import (
	"fmt"

	"github.com/Anykey-Nomad/Open-Kaurava/entities"
)

func main() {
	f := entities.NewFactory()
	if err := f.RegisterPlayer(entities.Player{ID: "player-1", Name: "Игрок", AllowedProduction: []entities.Kind{entities.KindUnit, entities.KindBuilding}}); err != nil {
		fmt.Println("register:", err)
		return
	}
	unit, err := f.Spawn(entities.Spec{Name: "builder", Kind: entities.KindUnit, Owner: "player-1", Speed: 10, HitPoints: 60}, entities.Position{X: 0, Y: 0})
	if err != nil {
		fmt.Println("spawn unit:", err)
		return
	}
	point, err := f.Spawn(entities.Spec{Name: "strategic_point", Kind: entities.KindGameplay, Owner: entities.NeutralPlayer, Capturable: true}, entities.Position{X: 25, Y: 0})
	if err != nil {
		fmt.Println("spawn point:", err)
		return
	}
	// Приказ: идти на точку и захватить её.
	if err := f.Move(unit.ID, entities.Order{
		Target: entities.Position{X: 25, Y: 0}, TargetID: point.ID, Stop: 0.5, Capture: true,
	}); err != nil {
		fmt.Println("move:", err)
		return
	}
	for i := 0; i < 300; i++ {
		f.Tick(0.016) // ~4.8 c — юниту нужно ~2.5 c, дальше захват
	}
	mid, _ := f.Get(unit.ID)
	fmt.Printf("после 4.8c: позиция=(%.2f, %.2f) статус=%v захват=%v прогр=%.0f%%\n",
		mid.Position.X, mid.Position.Y, mid.OrderStatus, mid.Capturing, mid.CaptureProgress*100)
	for i := 0; i < 600; i++ {
		f.Tick(0.016) // до ~14.4 c — захват идёт CaptureSeconds (10 c)
	}
	e, _ := f.Get(unit.ID)
	fmt.Printf("после 14.4c: позиция=(%.2f, %.2f) приказ=%v статус=%v жив=%v хп=%.0f/%.0f скорость=%.0f\n",
		e.Position.X, e.Position.Y, e.Order.Target, e.OrderStatus, e.Alive, e.Health.Current, e.Health.Max, e.Speed)
	pt, _ := f.Get(point.ID)
	fmt.Println("владелец точки:", pt.Owner)

	// Здание без скорости приказ получить не может.
	building, err := f.Spawn(entities.Spec{Name: "hq", Kind: entities.KindBuilding, Owner: "player-1"}, entities.Position{X: 1, Y: 1})
	if err != nil {
		fmt.Println("spawn building:", err)
		return
	}
	err = f.Move(building.ID, entities.Order{Target: entities.Position{X: 5, Y: 5}})
	fmt.Println("приказ зданию:", err)

	// Спешка не даёт юниту пролететь цель: идём на 50 при Stop=20.
	run, _ := f.Spawn(entities.Spec{Name: "runner", Kind: entities.KindUnit, Owner: "player-1", Speed: 100}, entities.Position{X: 0, Y: 100})
	_ = f.Move(run.ID, entities.Order{Target: entities.Position{X: 50, Y: 100}, Stop: 20})
	for i := 0; i < 10; i++ {
		f.Tick(0.1)
	}
	r, _ := f.Get(run.ID)
	fmt.Printf("быстрый бегун после 1c: позиция=(%.2f, %.2f) статус=%v (остановка на 20 от цели)\n", r.Position.X, r.Position.Y, r.OrderStatus)

	// Разгон и торможение по чертежу: Speed=10, AccelTime=2 (a=5 м/с²),
	// BrakeTime=1 (b=10 м/с²). Идеал: за 0.5 c — 0.625 м, за 1 c — 2.5 м
	// (не 5 и не 10), полная скорость набирается к 2 c; тормозной путь
	// на 10 м/с — 5 м, поэтому юнит подкатывает к цели, а не долетает.
	ramp, _ := f.Spawn(entities.Spec{Name: "ramp", Kind: entities.KindUnit, Owner: "player-1", Speed: 10, AccelTime: 2, BrakeTime: 1}, entities.Position{X: 0, Y: 300})
	_ = f.Move(ramp.ID, entities.Order{Target: entities.Position{X: 30, Y: 300}, Stop: 0.2})
	elapsed := 0.0
	for _, sample := range []struct {
		until float64
		note  string
	}{{0.5, "ожидается ~0.6 м"}, {1.0, "ожидается ~2.5 м"}, {2.0, "ожидается ~10 м — скорость набрана"}, {4.6, "прибытие с торможением: ~29.8 м"}} {
		for elapsed < sample.until {
			f.Tick(0.016)
			elapsed += 0.016
		}
		rr, _ := f.Get(ramp.ID)
		fmt.Printf("разгон через %.1fc: позиция=%.2fм (%s) скорость=%.2f статус=%v\n",
			sample.until, rr.Position.X, sample.note, rr.Velocity, rr.OrderStatus)
	}

	// Труп: жив=false, хп обнулены, приказ движения не принимает.
	corpse, _ := f.Spawn(entities.Spec{Name: "necron_lord", Kind: entities.KindUnit, Owner: "player-1", Speed: 10, HitPoints: 80, Dead: true}, entities.Position{X: 0, Y: 200})
	c, _ := f.Get(corpse.ID)
	fmt.Printf("труп: жив=%v хп=%.0f/%.0f, приказ: ", c.Alive, c.Health.Current, c.Health.Max)
	fmt.Println(f.Move(corpse.ID, entities.Order{Target: entities.Position{X: 10, Y: 200}}))
}

package fx

import (
	"math"
	"testing"
)

// TestParticlesDieOfAge — частица живёт ровно свой срок.
func TestParticlesDieOfAge(t *testing.T) {
	s := NewSeeded(1)
	s.SpawnFlash([3]float64{0, 0, 1})
	if s.Len() != 1 {
		t.Fatalf("частиц %d, ожидалась 1", s.Len())
	}
	s.Update(0.05)
	if s.Len() != 1 {
		t.Fatalf("вспышка умерла раньше срока: %d", s.Len())
	}
	s.Update(0.2)
	if s.Len() != 0 {
		t.Fatalf("вспышка не погасла: %d частиц", s.Len())
	}
}

// TestSmokeRisesAndGrows — дым поднимается и расширяется.
func TestSmokeRisesAndGrows(t *testing.T) {
	s := NewSeeded(2)
	s.SpawnSmoke([3]float64{10, 10, 0}, 2)
	start := s.Particles()[0]
	s.Update(1.0)
	p := s.Particles()[0]
	if p.Pos[2] <= start.Pos[2] {
		t.Fatalf("дым не поднялся: было %g, стало %g", start.Pos[2], p.Pos[2])
	}
	if p.SizeAt() <= start.SizeAt() {
		t.Fatalf("дым не расширился: было %g, стало %g", start.SizeAt(), p.SizeAt())
	}
	if p.ColorAt()[3] >= start.ColorAt()[3] {
		t.Fatalf("альфа дыма не гаснет: было %g, стало %g", start.ColorAt()[3], p.ColorAt()[3])
	}
}

// TestParticleCap — система не растёт бесконечно.
func TestParticleCap(t *testing.T) {
	s := NewSeeded(3)
	for i := 0; i < MaxParticles+300; i++ {
		s.SpawnSmoke([3]float64{0, 0, 0}, 1)
	}
	if s.Len() > MaxParticles {
		t.Fatalf("частиц %d при пределе %d", s.Len(), MaxParticles)
	}
}

// TestDirTo — направление считается единичным, совпавшие точки дают ноль.
func TestDirTo(t *testing.T) {
	d := DirTo([3]float64{0, 0, 0}, [3]float64{3, 0, 4})
	if math.Abs(d[0]-0.6) > 1e-9 || math.Abs(d[2]-0.8) > 1e-9 {
		t.Fatalf("направление %v, ожидалось (0.6, 0, 0.8)", d)
	}
	if zero := DirTo([3]float64{1, 1, 1}, [3]float64{1, 1, 1}); zero != [3]float64{} {
		t.Fatalf("совпавшие точки дали %v", zero)
	}
}

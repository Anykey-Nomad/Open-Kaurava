package scenes

import "testing"

// TestMonolithBlueprintDetection — пробуждение узнаётся по чертежам:
// монолит-постройка и восстановленный монолит-юнит отличаются, а обычные
// юниты и постройки под правило не попадают.
func TestMonolithBlueprintDetection(t *testing.T) {
	structures := map[string]bool{
		`races/necrons/structures/monolith`:              true,
		`races\necrons\structures\monolith.rgd`:          true,
		`races/necrons/structures/monolith_sp_dxp3`:      true,
		`races/necrons/troops/necron_restored_monolith`:  false,
		`races/necrons/troops/necron_immortal`:           false,
		`races/space_marines/structures/space_marine_hq`: false,
		"": false,
	}
	for blueprint, want := range structures {
		if got := isMonolithStructure(blueprint); got != want {
			t.Errorf("isMonolithStructure(%q) = %v, ожидалось %v", blueprint, got, want)
		}
	}
	units := map[string]bool{
		`races/necrons/troops/necron_restored_monolith`:     true,
		`races\necrons\troops\necron_restored_monolith.rgd`: true,
		`races/necrons/troops/necron_restored_monolith_gi`:  true,
		`races/necrons/troops/necron_immortal`:              false,
		`races/necrons/structures/monolith`:                 false,
	}
	for blueprint, want := range units {
		if got := isAwakenedMonolithUnit(blueprint); got != want {
			t.Errorf("isAwakenedMonolithUnit(%q) = %v, ожидалось %v", blueprint, got, want)
		}
	}
}

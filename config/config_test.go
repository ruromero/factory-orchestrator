package config

import (
	"testing"
	"time"
)

func TestPhaseDuration(t *testing.T) {
	tests := []struct {
		name  string
		cfg   Config
		phase string
		want  time.Duration
	}{
		{
			name:  "default when nothing configured",
			cfg:   Config{},
			phase: "coder",
			want:  15 * time.Minute,
		},
		{
			name: "uses global max_phase_duration",
			cfg: Config{
				MaxPhaseDuration: Duration{20 * time.Minute},
			},
			phase: "planner",
			want:  20 * time.Minute,
		},
		{
			name: "per-phase override takes precedence",
			cfg: Config{
				MaxPhaseDuration: Duration{15 * time.Minute},
				PhaseDurations: map[string]Duration{
					"coder": {30 * time.Minute},
				},
			},
			phase: "coder",
			want:  30 * time.Minute,
		},
		{
			name: "falls back to global when phase not in map",
			cfg: Config{
				MaxPhaseDuration: Duration{20 * time.Minute},
				PhaseDurations: map[string]Duration{
					"coder": {30 * time.Minute},
				},
			},
			phase: "planner",
			want:  20 * time.Minute,
		},
		{
			name: "zero per-phase duration falls back to global",
			cfg: Config{
				MaxPhaseDuration: Duration{20 * time.Minute},
				PhaseDurations: map[string]Duration{
					"coder": {0},
				},
			},
			phase: "coder",
			want:  20 * time.Minute,
		},
		{
			name: "zero global falls back to hardcoded default",
			cfg: Config{
				MaxPhaseDuration: Duration{0},
			},
			phase: "gatherer",
			want:  15 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.PhaseDuration(tt.phase)
			if got != tt.want {
				t.Errorf("PhaseDuration(%q) = %v, want %v", tt.phase, got, tt.want)
			}
		})
	}
}

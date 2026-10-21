package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// GenerateFauxRecord builds a comprehensive faux record with facts across all
// live facet types mentioned in card 210.
func GenerateFauxRecord(ctx context.Context, l *ledger.Ledger, g *gate.Gate, clk *stepClock, progress func(string)) error {
	if progress != nil {
		progress("generating comprehensive faux record...")
	}
	
	// Create a deterministic random number generator
	rng := rand.New(rand.NewSource(42)) // Fixed seed for deterministic generation
	
	// Generate facts for different facet types
	
	// 1. Generate blood pressure readings
	if progress != nil {
		progress("generating blood pressure readings...")
	}
	bpFacts := generateBloodPressureFacts(rng, 1000)
	if err := admitFauxFacts(ctx, g, clk, bpFacts, progress, "blood-pressure"); err != nil {
		return fmt.Errorf("generating blood pressure facts: %w", err)
	}
	
	// 2. Generate weight measurements
	if progress != nil {
		progress("generating weight measurements...")
	}
	weightFacts := generateWeightFacts(rng, 200)
	if err := admitFauxFacts(ctx, g, clk, weightFacts, progress, "weight"); err != nil {
		return fmt.Errorf("generating weight facts: %w", err)
	}
	
	// 3. Generate sleep episodes
	if progress != nil {
		progress("generating sleep episodes...")
	}
	sleepFacts := generateSleepFacts(rng, 300)
	if err := admitFauxFacts(ctx, g, clk, sleepFacts, progress, "sleep"); err != nil {
		return fmt.Errorf("generating sleep facts: %w", err)
	}
	
	// 4. Generate step counts
	if progress != nil {
		progress("generating step counts...")
	}
	stepsFacts := generateStepsFacts(rng, 500)
	if err := admitFauxFacts(ctx, g, clk, stepsFacts, progress, "steps"); err != nil {
		return fmt.Errorf("generating steps facts: %w", err)
	}
	
	// 5. Generate heart rate data
	if progress != nil {
		progress("generating heart rate data...")
	}
	hrFacts := generateHeartRateFacts(rng, 400)
	if err := admitFauxFacts(ctx, g, clk, hrFacts, progress, "heart-rate"); err != nil {
		return fmt.Errorf("generating heart rate facts: %w", err)
	}
	
	// 6. Generate exercise sessions
	if progress != nil {
		progress("generating exercise sessions...")
	}
	exerciseFacts := generateExerciseSessionFacts(rng, 150)
	if err := admitFauxFacts(ctx, g, clk, exerciseFacts, progress, "exercise"); err != nil {
		return fmt.Errorf("generating exercise facts: %w", err)
	}
	
	// 7. Generate lab results
	if progress != nil {
		progress("generating lab results...")
	}
	labFacts := generateLabFacts(rng, 50)
	if err := admitFauxFacts(ctx, g, clk, labFacts, progress, "lab"); err != nil {
		return fmt.Errorf("generating lab facts: %w", err)
	}
	
	// 8. Generate trip data
	if progress != nil {
		progress("generating trip data...")
	}
	tripFacts := generateTripFacts(rng, 20)
	if err := admitFauxFacts(ctx, g, clk, tripFacts, progress, "trip"); err != nil {
		return fmt.Errorf("generating trip facts: %w", err)
	}
	
	// 9. Generate medication data
	if progress != nil {
		progress("generating medication data...")
	}
	medFacts := generateMedicationFacts(rng, 30)
	if err := admitFauxFacts(ctx, g, clk, medFacts, progress, "medication"); err != nil {
		return fmt.Errorf("generating medication facts: %w", err)
	}
	
	// TODO: Add more facet types as needed
	
	return nil
}

// fauxFact represents a fact to be generated
type fauxFact struct {
	envelope facet.Envelope
}

// generateBloodPressureFacts generates blood pressure readings
func generateBloodPressureFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		systolic := 110 + rng.Intn(50)
		diastolic := 70 + rng.Intn(30)
		
		payload, _ := json.Marshal(map[string]interface{}{
			"date_excel": date.Format("2006-1-2 15:04:05"),
			"reading":    fmt.Sprintf("reading-%d", i),
			"systolic_blood_pressure": map[string]interface{}{
				"value": systolic,
				"unit":  "mmHg",
			},
			"diastolic_blood_pressure": map[string]interface{}{
				"value": diastolic,
				"unit":  "mmHg",
			},
			"effective_time_frame": map[string]string{
				"date_time": date.Format(time.RFC3339),
			},
		})
		
		env := facet.Envelope{
			Type:            "health.blood-pressure",
			Source:          "bp-csv",
			Injector:        "bp-csv",
			SourceRef:       facet.SourceRef("bp-csv", date.Format("2006-1-2 15:04:05"), fmt.Sprintf("reading-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateWeightFacts generates weight measurements
func generateWeightFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		weight := 70.0 + rng.Float64()*20.0 // 70-90 kg
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id":   fmt.Sprintf("weight-%d", i),
			"date": date.Format("2006-01-02 15:04"),
			"kg":   weight,
		})
		
		env := facet.Envelope{
			Type:            "health.weight",
			Source:          "withings",
			Injector:        "withings",
			SourceRef:       facet.SourceRef("withings", fmt.Sprintf("weight-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateSleepFacts generates sleep episodes
func generateSleepFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		duration := 240 + rng.Intn(240) // 4-8 hours in minutes
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("sleep-%d", i),
			"effective_time_frame": map[string]interface{}{
				"time_interval": map[string]interface{}{
					"start_date_time": date.Format(time.RFC3339),
					"end_date_time":   date.Add(time.Duration(duration) * time.Minute).Format(time.RFC3339),
				},
			},
			"source_time": map[string]interface{}{
				"value": date.Format("2006-01-02 15:04:05"),
				"zone":  "Australia/Sydney",
				"basis": "source",
			},
		})
		
		env := facet.Envelope{
			Type:            "health.sleep-episode",
			Source:          "withings",
			Injector:        "withings",
			SourceRef:       facet.SourceRef("withings", fmt.Sprintf("sleep-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateStepsFacts generates step counts
func generateStepsFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		steps := 5000 + rng.Intn(10000) // 5000-15000 steps
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("steps-%d", i),
            "effective_time_frame": map[string]interface{}{
                "time_interval": map[string]interface{}{
                    "start_date_time": date.Format(time.RFC3339),
                    "end_date_time":   date.Add(24 * time.Hour).Format(time.RFC3339),
                },
            },
            "source_time": map[string]interface{}{
                "value": date.Format("2006-01-02 15:04:05"),
                "zone":  "Australia/Sydney",
                "basis": "source",
            },
            "step_count": map[string]interface{}{
                "value": steps,
                "unit":  "steps",
            },
			Injector:        "garmin",
			SourceRef:       facet.SourceRef("garmin", fmt.Sprintf("steps-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateHeartRateFacts generates heart rate data
func generateHeartRateFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		restingHR := 60 + rng.Intn(20) // 60-80 bpm
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("hr-%d", i),
			"effective_time_frame": map[string]string{
				"date": date.Format("2006-01-02 15:04"),
			},
			"heart_rate": map[string]interface{}{
				"value": restingHR,
				"unit":  "bpm",
			},
		})
		
		env := facet.Envelope{
			Type:            "health.heart-rate.day",
			Source:          "garmin",
			Injector:        "garmin",
			SourceRef:       facet.SourceRef("garmin", fmt.Sprintf("hr-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateExerciseSessionFacts generates exercise sessions
func generateExerciseSessionFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	sources := []string{"garmin", "concept2", "technogym", "strong"}
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		source := sources[rng.Intn(len(sources))]
		duration := 30 + rng.Intn(90) // 30-120 minutes
		calories := 200 + rng.Intn(600) // 200-800 calories
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("exercise-%d", i),
			"effective_time_frame": map[string]interface{}{
				"time_interval": map[string]string{
					"start_date_time": date.Format(time.RFC3339),
					"end_date_time":  date.Add(time.Duration(duration) * time.Minute).Format(time.RFC3339),
				},
			},
			"activity_name": "Running",
			"distance": map[string]interface{}{
				"value": float64(duration) * 0.2, // km
				"unit":  "km",
			},
			"kcal": calories,
		})
		
		env := facet.Envelope{
			Type:            facet.Type(fmt.Sprintf("activity.exercise-session.%s", source)),
			Source:          source,
			Injector:        source,
			SourceRef:       facet.SourceRef(source, fmt.Sprintf("exercise-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateLabFacts generates lab results
func generateLabFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		tests := []struct {
			name  string
			value float64
			unit  string
		}{
			{"Cholesterol", 150 + rng.Float64()*100, "mg/dL"},
			{"Glucose", 80 + rng.Float64()*40, "mg/dL"},
			{"Hemoglobin", 14 + rng.Float64()*2, "g/dL"},
		}
		
		test := tests[rng.Intn(len(tests))]
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("lab-%d", i),
			"collected": date.Format("2006-01-02"),
			"test_name": test.name,
			"value": map[string]interface{}{
				"value": test.value,
				"unit":  test.unit,
			},
		})
		
		env := facet.Envelope{
			Type:            "health.lab-result",
			Source:          "pathology",
			Injector:        "pathology",
			SourceRef:       facet.SourceRef("pathology", fmt.Sprintf("lab-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateTripFacts generates trip data
func generateTripFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("trip-%d", i),
			"date": date.Format("2006-01-02 15:04"),
		})
		
		env := facet.Envelope{
			Type:            "place.trip.departed",
			Source:          "travel-app",
			Injector:        "travel-app",
			SourceRef:       facet.SourceRef("travel-app", fmt.Sprintf("trip-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// generateMedicationFacts generates medication data
func generateMedicationFacts(rng *rand.Rand, count int) []fauxFact {
	var facts []fauxFact
	medications := []string{"Aspirin", "Metformin", "Lisinopril", "Atorvastatin"}
	
	for i := 0; i < count; i++ {
		daysAgo := rng.Intn(365*2) // 2 years of data
		date := time.Now().AddDate(0, 0, -daysAgo)
		
		med := medications[rng.Intn(len(medications))]
		dose := 50 + rng.Intn(150) // 50-200 mg
		
		payload, _ := json.Marshal(map[string]interface{}{
			"id": fmt.Sprintf("med-%d", i),
			"date": date.Format("2006-01-02 15:04"),
			"medication": med,
			"dose": map[string]interface{}{
				"value": dose,
				"unit":  "mg",
			},
		})
		
		env := facet.Envelope{
			Type:            "health.medication.dose",
			Source:          "med-app",
			Injector:        "med-app",
			SourceRef:       facet.SourceRef("med-app", fmt.Sprintf("med-%d", i)),
			Payload:         payload,
			AssertedPerson:  Person,
		}
		
		facts = append(facts, fauxFact{envelope: env})
	}
	
	return facts
}

// admitFauxFacts admits the generated facts through the gate
func admitFauxFacts(ctx context.Context, g *gate.Gate, clk *stepClock, facts []fauxFact, progress func(string), label string) error {
	for i, f := range facts {
		if progress != nil && i%50 == 0 {
			progress(fmt.Sprintf("admitting %s fact %d/%d", label, i, len(facts)))
		}
		
		// Set timestamp
		clk.now = time.Now()
		
		// Resolve and commit
		resolved, err := g.Resolve(f.envelope)
		if err != nil {
			return fmt.Errorf("resolving envelope %d: %w", i, err)
		}
		
		_, _, err = g.Commit(ctx, f.envelope, resolved)
		if err != nil {
			return fmt.Errorf("committing envelope %d: %w", i, err)
		}
	}
	
	return nil
}
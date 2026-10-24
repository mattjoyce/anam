package gate_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 241: money.purchase over the bundled Definition, a real gate and a real ledger.
// The merchant and the orders here are made up. One order is one facet whose id the
// Definition composes from the merchant and the order reference, and the person's
// judgement about it (deductible, purpose) has no field in it.

const purchaseType = "money.purchase"

func newPurchaseGate(t *testing.T) (*gate.Gate, *ledger.Ledger, *def.Registry) {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	scopes := gate.StaticScopes{"records": {purchaseType}}
	g := gate.New(l, reg, scopes, clock.Real{}, person, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	return g, l, reg
}

func sayPurchase(t *testing.T, g *gate.Gate, payload string) (gate.Result, error) {
	t.Helper()
	env := facet.Envelope{Type: purchaseType, Source: "records", Injector: "records", Payload: []byte(payload)}
	b, err := g.Resolve(env)
	if err != nil {
		return gate.Result{}, err
	}
	res, _, err := g.Commit(context.Background(), env, b)
	return res, err
}

// examplePurchase is a whole order shaped like a real invoice (one line, shipping, tax, a
// payment method), with fields replacing or adding to the defaults. A field set to nil is
// left out.
func examplePurchase(over map[string]string) string {
	f := map[string]string{
		"date":                  `"2026-10-24"`,
		"merchant":              `"example-shop"`,
		"merchant_legal_entity": `"Example Shop Pte. Ltd."`,
		"order_ref":             `"ORD-EX1042-0A1"`,
		"invoice_ref":           `"INV-EX0731-0A1"`,
		"items":                 `[{"name": "Example key", "sku": "0000000000001", "quantity": 1, "unit_price": 68.00, "line_total": 68.00}]`,
		"subtotal":              `68.00`,
		"shipping":              `10.00`,
		"tax":                   `7.80`,
		"tax_label":             `"Tax"`,
		"total":                 `85.80`,
		"currency":              `"USD"`,
		"paid_via":              `"paypal"`,
		"basis":                 `"order confirmation and invoice emails; payment receipt for USD 85.80"`,
	}
	for k, v := range over {
		if v == "" {
			delete(f, k)
		} else {
			f[k] = v
		}
	}
	var parts []string
	for k, v := range f {
		parts = append(parts, fmt.Sprintf("%q: %s", k, v))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func TestPurchase_AnOrderIsAdmittedUnderTheIDTheTemplateComposes(t *testing.T) {
	t.Parallel()
	g, _, _ := newPurchaseGate(t)
	const id = "records:purchase/example-shop/ORD-EX1042-0A1"
	want := facet.NewID(facet.SourceRef("records", id))

	// The id left out is filled by Anam.
	res, err := sayPurchase(t, g, examplePurchase(nil))
	if err != nil || res.Outcome != gate.OutcomeAdmitted {
		t.Fatalf("got %+v, %v; want admitted", res, err)
	}
	if res.FacetID != want {
		t.Errorf("facet id %s, want the one for %s (%s)", res.FacetID, id, want)
	}
	// The same payload with the id sent, as the template composes it, is the same
	// facet and a no-op.
	again, err := sayPurchase(t, g, examplePurchase(map[string]string{"id": `"` + id + `"`}))
	if err != nil || again.Outcome != gate.OutcomeNoOp || again.FacetID != res.FacetID {
		t.Errorf("saying the order with its id = %+v, %v; want a no-op on %s", again, err, res.FacetID)
	}
}

// The id is composed from the merchant and the order reference, so a merchant that is a
// slug and a reference that is verbatim make the id a writer reaches the same way every
// time, whatever else the order says.
func TestPurchase_TheIDIsComposedFromTheMerchantAndTheOrderReference(t *testing.T) {
	t.Parallel()
	g, _, _ := newPurchaseGate(t)
	a, err := sayPurchase(t, g, examplePurchase(nil))
	if err != nil {
		t.Fatal(err)
	}
	b, err := sayPurchase(t, g, examplePurchase(map[string]string{"order_ref": `"ORD-EX2077-0B2"`, "total": `20.00`}))
	if err != nil {
		t.Fatal(err)
	}
	if a.FacetID == b.FacetID || b.Outcome != gate.OutcomeAdmitted {
		t.Errorf("a second order from the same merchant: %+v after %+v, want a second facet", b, a)
	}
	if want := facet.NewID(facet.SourceRef("records", "records:purchase/example-shop/ORD-EX2077-0B2")); b.FacetID != want {
		t.Errorf("second order facet id %s, want %s", b.FacetID, want)
	}
	other, err := sayPurchase(t, g, examplePurchase(map[string]string{"merchant": `"other-shop"`}))
	if err != nil || other.FacetID == a.FacetID {
		t.Errorf("the same reference at another merchant = %+v, %v; want its own facet", other, err)
	}
}

func TestPurchase_AGoodOrderIsAdmittedAndABadOneRefused(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		over map[string]string
		ok   bool
	}{
		{"a whole order", nil, true},
		{"only what is required", map[string]string{
			"merchant_legal_entity": "", "invoice_ref": "", "items": "", "subtotal": "", "shipping": "", "tax": "", "tax_label": "", "paid_via": "", "basis": "",
		}, true},
		{"an order with a payment reference and a source note", map[string]string{"payment_ref": `"PAY-0001"`, "source_note": `"read from the PDF"`}, true},
		{"a line with no price, only a name", map[string]string{"items": `[{"name": "Example key"}]`}, true},
		{"a merchant with hyphens and digits", map[string]string{"merchant": `"amazon-au"`}, true},
		{"a wrong id", map[string]string{"id": `"records:purchase/example-shop/ORD-OTHER"`}, false},
		{"a merchant with spaces", map[string]string{"merchant": `"Example Shop"`}, false},
		{"a merchant in capitals", map[string]string{"merchant": `"Example-Shop"`}, false},
		{"a currency in lower case", map[string]string{"currency": `"usd"`}, false},
		{"a currency of four letters", map[string]string{"currency": `"USDT"`}, false},
		{"a currency symbol", map[string]string{"currency": `"$"`}, false},
		{"no order reference", map[string]string{"order_ref": ""}, false},
		{"an empty order reference", map[string]string{"order_ref": `""`}, false},
		{"no merchant", map[string]string{"merchant": ""}, false},
		{"no date", map[string]string{"date": ""}, false},
		{"no total", map[string]string{"total": ""}, false},
		{"a total as text", map[string]string{"total": `"85.80"`}, false},
		{"no currency", map[string]string{"currency": ""}, false},
		{"a date that is no day", map[string]string{"date": `"2026-02-30"`}, false},
		{"a date with a time", map[string]string{"date": `"2026-10-24 22:01"`}, false},
		{"a clock block on a date, which has none to state", map[string]string{"source_time": `{"value": "2026-10-24", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "assumption"}`}, false},
		{"a line with an unknown key", map[string]string{"items": `[{"name": "Example key", "colour": "blue"}]`}, false},
		{"a line with no name", map[string]string{"items": `[{"sku": "0000000000001", "quantity": 1}]`}, false},
		{"a line of zero", map[string]string{"items": `[{"name": "Example key", "quantity": 0}]`}, false},
		{"no lines at all, an empty list", map[string]string{"items": `[]`}, false},
		{"a negative shipping", map[string]string{"shipping": `-1`}, false},
		{"a deductible flag, which is the person's judgement", map[string]string{"deductible": `true`}, false},
		{"a purpose", map[string]string{"purpose": `"work"`}, false},
		{"a financial year", map[string]string{"financial_year": `"2026-27"`}, false},
		{"a converted total", map[string]string{"total_aud": `131.20`}, false},
		{"a payment method with spaces", map[string]string{"paid_via": `"Visa card"`}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g, l, _ := newPurchaseGate(t)
			res, err := sayPurchase(t, g, examplePurchase(tc.over))
			switch {
			case tc.ok && (err != nil || res.Outcome != gate.OutcomeAdmitted):
				t.Fatalf("got %+v, %v; want admitted", res, err)
			case !tc.ok && err == nil:
				t.Fatalf("admitted %+v; want a refusal", res)
			}
			if !tc.ok {
				t.Logf("refused: %v", err)
				got, err := fold.Read(context.Background(), l)
				if err != nil || len(got.Live) != 0 {
					t.Errorf("a refused payload left %d live facets (%v)", len(got.Live), err)
				}
			}
		})
	}
}

// The purchases projection serves the type and recent-life, the registered projection
// an assistant reads for daily life, does not: a purchase is Confidential and not part of
// that picture.
func TestPurchase_ThePurchasesProjectionServesItAndRecentLifeDoesNot(t *testing.T) {
	t.Parallel()
	g, l, reg := newPurchaseGate(t)
	one, err := sayPurchase(t, g, examplePurchase(nil))
	if err != nil {
		t.Fatal(err)
	}
	two, err := sayPurchase(t, g, examplePurchase(map[string]string{"order_ref": `"ORD-EX2077-0B2"`, "date": `"2026-11-02"`}))
	if err != nil {
		t.Fatal(err)
	}
	res, err := fold.Read(context.Background(), l)
	if err != nil {
		t.Fatal(err)
	}
	bundled, err := projection.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	purchases, err := bundled.Lookup("purchases")
	if err != nil {
		t.Fatal(err)
	}
	if purchases.Shape != "log" || purchases.MaxClassification != "Confidential" {
		t.Errorf("purchases = %s up to %s, want a log up to Confidential", purchases.Shape, purchases.MaxClassification)
	}
	served, err := projection.Apply(res.Value, purchases, consent.Open{}, reg)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range served.Entries() {
		got = append(got, e.FacetID)
	}
	if want := []string{one.FacetID, two.FacetID}; !sameIDs(got, want) {
		t.Errorf("purchases served %v, want both orders %v", got, want)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "projections", "recent-life.v6.json"))
	if err != nil {
		t.Fatal(err)
	}
	recent, err := projection.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if served, err = projection.Apply(res.Value, recent, consent.Open{}, reg); err != nil {
		t.Fatal(err)
	}
	if n := len(served.Entries()); n != 0 {
		t.Errorf("recent-life served %d purchases, want none", n)
	}
}

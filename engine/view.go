package engine

// ============================================================================
// RECORD VIEW — Zero-Copy Data Access Interface
// ============================================================================
// The engine never owns consumer data. It reads through this interface.
//
// Implementations:
//   SliceView      — wraps []Record (CSV, ad-hoc, backward compat)
//   DomainView[T]  — reads typed structs via accessor functions (zero-copy)
//   SubView        — filtered subset (indices into parent, zero-copy)
//   CurrencyView   — wraps any view, normalizes currency on read
//   ConcatView     — virtual concatenation of two views
//
// Consumers register accessors once at init; engine reads millions of times.
// ============================================================================

// RecordView provides indexed access to a dataset.
// The engine calls Dimension/Measure in tight loops — keep implementations fast.
type RecordView interface {
	Len() int
	Dimension(index int, key string) string
	Measure(index int, key string) float64
	DimensionKeys() []string // available dimension keys
	MeasureKeys() []string   // available measure keys
}

// ============================================================================
// SLICE VIEW — wraps []Record (backward compatible)
// ============================================================================

// SliceView wraps a []Record slice as a RecordView.
// Used by helpers.ParseCSV and ad-hoc consumers.
type SliceView struct {
	records []Record
	dimKeys []string
	mesKeys []string
}

// NewSliceView creates a RecordView from a []Record slice.
func NewSliceView(records []Record) RecordView {
	v := &SliceView{records: records}
	v.cacheKeys()
	return v
}

func (v *SliceView) cacheKeys() {
	if len(v.records) == 0 {
		return
	}
	dimSeen := make(map[string]bool)
	mesSeen := make(map[string]bool)
	for _, r := range v.records {
		for k := range r.Dimensions {
			if !dimSeen[k] {
				dimSeen[k] = true
				v.dimKeys = append(v.dimKeys, k)
			}
		}
		for k := range r.Measures {
			if !mesSeen[k] {
				mesSeen[k] = true
				v.mesKeys = append(v.mesKeys, k)
			}
		}
	}
}

func (v *SliceView) Len() int { return len(v.records) }

func (v *SliceView) Dimension(i int, key string) string {
	if i < 0 || i >= len(v.records) {
		return ""
	}
	return v.records[i].Dimensions[key]
}

func (v *SliceView) Measure(i int, key string) float64 {
	if i < 0 || i >= len(v.records) {
		return 0
	}
	return v.records[i].Measures[key]
}

func (v *SliceView) DimensionKeys() []string { return v.dimKeys }
func (v *SliceView) MeasureKeys() []string   { return v.mesKeys }

// ============================================================================
// SUB VIEW — filtered subset (zero-copy)
// ============================================================================

// SubView is a filtered subset of a parent RecordView.
// Holds indices into the parent — no data copy.
type SubView struct {
	parent  RecordView
	indices []int
}

func newSubView(parent RecordView, indices []int) RecordView {
	return &SubView{parent: parent, indices: indices}
}

func (v *SubView) Len() int { return len(v.indices) }

func (v *SubView) Dimension(i int, key string) string {
	if i < 0 || i >= len(v.indices) {
		return ""
	}
	return v.parent.Dimension(v.indices[i], key)
}

func (v *SubView) Measure(i int, key string) float64 {
	if i < 0 || i >= len(v.indices) {
		return 0
	}
	return v.parent.Measure(v.indices[i], key)
}

func (v *SubView) DimensionKeys() []string { return v.parent.DimensionKeys() }
func (v *SubView) MeasureKeys() []string   { return v.parent.MeasureKeys() }

// ============================================================================
// CONCAT VIEW — virtual concatenation of two views
// ============================================================================

// ConcatView logically concatenates two RecordViews.
// Used for ratio period derivation without data copy.
type ConcatView struct {
	a, b RecordView
}

func newConcatView(a, b RecordView) RecordView {
	return &ConcatView{a: a, b: b}
}

func (v *ConcatView) Len() int { return v.a.Len() + v.b.Len() }

func (v *ConcatView) Dimension(i int, key string) string {
	if i < v.a.Len() {
		return v.a.Dimension(i, key)
	}
	return v.b.Dimension(i-v.a.Len(), key)
}

func (v *ConcatView) Measure(i int, key string) float64 {
	if i < v.a.Len() {
		return v.a.Measure(i, key)
	}
	return v.b.Measure(i-v.a.Len(), key)
}

func (v *ConcatView) DimensionKeys() []string { return v.a.DimensionKeys() }
func (v *ConcatView) MeasureKeys() []string   { return v.a.MeasureKeys() }

// ============================================================================
// CURRENCY VIEW — on-read normalization (zero-copy)
// ============================================================================

// CurrencyView wraps a RecordView and normalizes currency on read.
// No data copy — conversion happens per Measure() call.
type CurrencyView struct {
	parent       RecordView
	measure      string
	dimension    string
	baseCurrency string
	rates        map[string]float64
}

func newCurrencyView(parent RecordView, measure, dimension, baseCurrency string, rates map[string]float64) RecordView {
	return &CurrencyView{
		parent:       parent,
		measure:      measure,
		dimension:    dimension,
		baseCurrency: baseCurrency,
		rates:        rates,
	}
}

func (v *CurrencyView) Len() int { return v.parent.Len() }

func (v *CurrencyView) Dimension(i int, key string) string {
	if key == v.dimension {
		orig := v.parent.Dimension(i, key)
		if orig != v.baseCurrency {
			if _, ok := v.rates[orig]; ok {
				return v.baseCurrency
			}
		}
		return orig
	}
	return v.parent.Dimension(i, key)
}

func (v *CurrencyView) Measure(i int, key string) float64 {
	val := v.parent.Measure(i, key)
	if key == v.measure {
		currency := v.parent.Dimension(i, v.dimension)
		if currency != v.baseCurrency {
			if rate, ok := v.rates[currency]; ok && rate > 0 {
				return val * rate
			}
		}
	}
	return val
}

func (v *CurrencyView) DimensionKeys() []string { return v.parent.DimensionKeys() }
func (v *CurrencyView) MeasureKeys() []string   { return v.parent.MeasureKeys() }

// ============================================================================
// DOMAIN ADAPTER — Zero-copy typed struct access
// ============================================================================
//
// Usage:
//
//	adapter := engine.NewDomainAdapter[Transaction]().
//	    Dimension("category", func(t Transaction) string { return t.CategoryName }).
//	    Measure("amount", func(t Transaction) float64 { return t.Amount })
//
//	view := adapter.Bind(transactions)
//	result, _ := engine.Execute(spec, view, opts...)
//
// ============================================================================

// DomainAdapter builds a RecordView from typed structs.
// Declare once, bind many times.
type DomainAdapter[T any] struct {
	dimOrder []string
	mesOrder []string
	dims     map[string]func(T) string
	meas     map[string]func(T) float64
}

// NewDomainAdapter creates a new adapter for type T.
func NewDomainAdapter[T any]() *DomainAdapter[T] {
	return &DomainAdapter[T]{
		dims: make(map[string]func(T) string),
		meas: make(map[string]func(T) float64),
	}
}

// Dimension registers a dimension accessor.
func (a *DomainAdapter[T]) Dimension(key string, fn func(T) string) *DomainAdapter[T] {
	if _, exists := a.dims[key]; !exists {
		a.dimOrder = append(a.dimOrder, key)
	}
	a.dims[key] = fn
	return a
}

// Measure registers a measure accessor.
func (a *DomainAdapter[T]) Measure(key string, fn func(T) float64) *DomainAdapter[T] {
	if _, exists := a.meas[key]; !exists {
		a.mesOrder = append(a.mesOrder, key)
	}
	a.meas[key] = fn
	return a
}

// Bind creates a RecordView from a data slice. Zero-copy — holds reference.
func (a *DomainAdapter[T]) Bind(data []T) RecordView {
	return &DomainView[T]{
		data:     data,
		dims:     a.dims,
		meas:     a.meas,
		dimKeys:  a.dimOrder,
		measKeys: a.mesOrder,
	}
}

// DomainView reads typed struct fields via registered accessor functions.
type DomainView[T any] struct {
	data     []T
	dims     map[string]func(T) string
	meas     map[string]func(T) float64
	dimKeys  []string
	measKeys []string
}

func (v *DomainView[T]) Len() int { return len(v.data) }

func (v *DomainView[T]) Dimension(i int, key string) string {
	if i < 0 || i >= len(v.data) {
		return ""
	}
	if fn, ok := v.dims[key]; ok {
		return fn(v.data[i])
	}
	return ""
}

func (v *DomainView[T]) Measure(i int, key string) float64 {
	if i < 0 || i >= len(v.data) {
		return 0
	}
	if fn, ok := v.meas[key]; ok {
		return fn(v.data[i])
	}
	return 0
}

func (v *DomainView[T]) DimensionKeys() []string { return v.dimKeys }
func (v *DomainView[T]) MeasureKeys() []string   { return v.measKeys }
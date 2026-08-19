package main

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/garnizeh/cinzal/internal/game"
)

// sweepFlags accumulates repeated --sweep Field=v1,v2,... flags in the
// order given on the command line. That order is what makes the CSV's row
// order — and D35's "read [the sweep points] in dial order" — reproducible
// from the invocation alone.
type sweepFlags []string

func (s *sweepFlags) String() string { return strings.Join(*s, " ") }

func (s *sweepFlags) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// sweepDim is one --sweep flag, validated against game.Config's own
// reflected shape: a field that exists, has a kind --sweep can set, and a
// value list every entry of which parses into that kind. This validation
// happens once, for every dimension, before any match runs — issue #200's
// own acceptance criterion is that an unknown field name "fails before the
// first match runs," not partway through a nine-hour sweep.
type sweepDim struct {
	field  string
	kind   reflect.Kind
	values []any // each element's concrete type matches kind
}

// parseSweepFlags validates raw's "Field=v1,v2,..." entries against
// game.Config's reflected type.
//
// Only scalar kinds are settable this way (Int, Int64, Float64, Bool,
// String) — every one of game.Config's current scalar fields is a plain
// int, so in practice today this accepts exactly those. A field of
// composite kind (StepsByTier, Contracts, MapByPlayers, Scavenging,
// Pressure, Suppress — arrays, maps, and structs) is rejected with a
// message naming the field and its kind: nothing in RFC §16.4's worked
// example or issue #200 asks for a path/index syntax into a composite
// field, and inventing one here would be a bigger decision than this task
// was asked to make.
//
// A hand-written name→setter table was the other option issue #200
// weighed for this: less runtime risk, but it's more code and a renamed
// Config field is a silent runtime miss instead of a compile error. This
// task's own judgement (recorded here, per the issue's own request) is
// that reflection is the right tool as long as it fails before the first
// match runs — which this function, called once at startup, guarantees.
func parseSweepFlags(raw []string) ([]sweepDim, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("cmd/simulate: --sweep: at least one --sweep Field=v1,v2,... flag is required")
	}

	cfgType := reflect.TypeFor[game.Config]()

	dims := make([]sweepDim, 0, len(raw))
	for _, r := range raw {
		field, valuesRaw, ok := strings.Cut(r, "=")
		if !ok || field == "" {
			return nil, fmt.Errorf("cmd/simulate: --sweep %q: want the form Field=v1,v2", r)
		}
		if valuesRaw == "" {
			return nil, fmt.Errorf("cmd/simulate: --sweep %q: no values after '='", r)
		}

		sf, ok := cfgType.FieldByName(field)
		if !ok {
			return nil, fmt.Errorf("cmd/simulate: --sweep %q: game.Config has no field %q", r, field)
		}
		if !supportedSweepKind(sf.Type.Kind()) {
			return nil, fmt.Errorf("cmd/simulate: --sweep %q: game.Config.%s has kind %s, which --sweep cannot set (only scalar int/float/bool/string fields are sweepable)", r, field, sf.Type.Kind())
		}

		values := make([]any, 0)
		for vs := range strings.SplitSeq(valuesRaw, ",") {
			v, err := parseScalar(vs, sf.Type.Kind())
			if err != nil {
				return nil, fmt.Errorf("cmd/simulate: --sweep %q: value %q: %w", r, vs, err)
			}
			values = append(values, v)
		}

		dims = append(dims, sweepDim{field: field, kind: sf.Type.Kind(), values: values})
	}

	return dims, nil
}

func supportedSweepKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int64, reflect.Float64, reflect.Bool, reflect.String:
		return true
	default:
		return false
	}
}

func parseScalar(s string, k reflect.Kind) (any, error) {
	switch k {
	case reflect.Int:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case reflect.Int64:
		return strconv.ParseInt(s, 10, 64)
	case reflect.Float64:
		return strconv.ParseFloat(s, 64)
	case reflect.Bool:
		return strconv.ParseBool(s)
	case reflect.String:
		return s, nil
	default:
		panic(fmt.Sprintf("cmd/simulate: parseScalar: unsupported kind %s", k))
	}
}

// formatScalar is parseScalar's inverse, used everywhere a swept value is
// rendered back out — the CSV's "sweep.<Field>" column, the resume skip
// key, and stderr progress lines all call this one function so the three
// can never drift out of the string agreement resume-matching depends on.
func formatScalar(v any) string {
	switch x := v.(type) {
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case string:
		return x
	default:
		panic(fmt.Sprintf("cmd/simulate: formatScalar: unsupported type %T", v))
	}
}

// configPoint is one cell of the sweep's cartesian product: the swept
// field values that produced it, parallel to the []sweepDim that generated
// it (so a caller renders "sweep.<Field>" columns without re-deriving them
// from cfg by reflection a second time), and the full game.Config those
// values were applied to.
type configPoint struct {
	values []any
	cfg    game.Config
}

// expandSweep builds the ordered cartesian product of dims over
// game.DefaultConfig(), applying each dimension's field=value by
// reflection. dims[0]'s values vary slowest, matching how
// "--sweep A=1,2 --sweep B=3,4" reads left to right: (1,3), (1,4), (2,3),
// (2,4). This order is what row order in the CSV follows, and it is fixed
// by the command line alone — nothing here depends on iteration order over
// a map.
func expandSweep(dims []sweepDim) []configPoint {
	points := []configPoint{{cfg: game.DefaultConfig()}}
	for _, d := range dims {
		next := make([]configPoint, 0, len(points)*len(d.values))
		for _, p := range points {
			for _, v := range d.values {
				cfg := p.cfg
				setConfigField(&cfg, d.field, v)
				values := append(slices.Clone(p.values), v)
				next = append(next, configPoint{values: values, cfg: cfg})
			}
		}
		points = next
	}
	return points
}

func setConfigField(cfg *game.Config, field string, v any) {
	reflect.ValueOf(cfg).Elem().FieldByName(field).Set(reflect.ValueOf(v))
}

// sweepSpecString renders dims into the canonical form recorded in the
// CSV's provenance line and compared verbatim on resume:
// "Field1:v1,v2;Field2:v3,v4".
func sweepSpecString(dims []sweepDim) string {
	parts := make([]string, len(dims))
	for i, d := range dims {
		vs := make([]string, len(d.values))
		for j, v := range d.values {
			vs[j] = formatScalar(v)
		}
		parts[i] = d.field + ":" + strings.Join(vs, ",")
	}
	return strings.Join(parts, ";")
}

// sweepValuesString renders one configPoint's own values against dims, for
// stderr progress lines: "LeaseCostPerBlock=3".
func sweepValuesString(dims []sweepDim, p configPoint) string {
	parts := make([]string, len(dims))
	for i, d := range dims {
		parts[i] = d.field + "=" + formatScalar(p.values[i])
	}
	return strings.Join(parts, " ")
}

// configKey is the resume skip-key for a configPoint — the same swept
// values, in the same order, that csvReport reconstructs from an existing
// file's "sweep.<Field>" columns (see csv_report.go's existingRowKey),
// joined with a separator that cannot appear in a formatted scalar.
func configKey(p configPoint) string {
	parts := make([]string, len(p.values))
	for i, v := range p.values {
		parts[i] = formatScalar(v)
	}
	return strings.Join(parts, "\x1f")
}

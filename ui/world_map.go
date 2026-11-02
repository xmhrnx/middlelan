package ui

import (
	"encoding/binary"
	"math"
	"sync"

	"github.com/xmhrnx/giode"
	props "github.com/xmhrnx/giode/styles/properties"
	"github.com/xmhrnx/middlelan/assets"
)

// landMask is the decoded assets.World grid.
type landMask struct {
	cols, rows   int
	north, south float64
	bits         []byte
}

var (
	worldOnce sync.Once
	world     landMask
)

func loadWorld() landMask {
	worldOnce.Do(func() {
		b := assets.World
		if len(b) < 8 {
			return
		}
		world = landMask{
			cols: int(binary.LittleEndian.Uint16(b[0:])), rows: int(binary.LittleEndian.Uint16(b[2:])),
			north: float64(int16(binary.LittleEndian.Uint16(b[4:]))) / 10, south: float64(int16(binary.LittleEndian.Uint16(b[6:]))) / 10,
			bits: b[8:],
		}
	})
	return world
}

func (m landMask) land(r, c int) bool {
	i := r*m.cols + c
	return i/8 < len(m.bits) && m.bits[i/8]>>(i%8)&1 == 1
}

// project maps a coordinate to the canvas.
func (m landMask) project(lat, lon float64, w, h float32) (float32, float32) {
	x := float32((lon+180)/360) * w
	y := float32((m.north-lat)/(m.north-m.south)) * h
	return x, y
}

// mapMarker is a place to mark on the map.
type mapMarker struct {
	lat, lon float64
	weight   float64 // 0..1, sizes the marker
	color    giode.Color
}

// worldMap draws the continents as a dot matrix, the markers, and arcs
// from home (when known) to each marker. The land layer is memoized: it
// only redraws when the size changes.
func worldMap(markers []mapMarker, home *mapMarker) giode.Element {
	m := loadWorld()
	if m.cols == 0 {
		return emptyState("map", tr("Map unavailable"), tr("The map data couldn't be loaded."), nil)
	}
	aspect := float32(m.cols) / float32(m.rows)
	land := giode.Memo("world-land", func() giode.Element {
		return giode.Canvas(giode.Styles{Width: -1, AspectRatio: props.AspectRatio(aspect)}, func(p *giode.Painter) {
			w, h := p.Width(), p.Height()
			cw, ch := w/float32(m.cols), h/float32(m.rows)
			r := min(cw, ch) * 0.3
			for row := 0; row < m.rows; row++ {
				for col := 0; col < m.cols; col++ {
					if m.land(row, col) {
						p.Circle((float32(col)+0.5)*cw, (float32(row)+0.5)*ch, r, colMapLand)
					}
				}
			}
		})
	})
	overlay := giode.Canvas(giode.Styles{Width: -1, AspectRatio: props.AspectRatio(aspect)}, func(p *giode.Painter) {
		w, h := p.Width(), p.Height()
		var hx, hy float32
		if home != nil {
			hx, hy = m.project(home.lat, home.lon, w, h)
		}
		for _, mk := range markers {
			x, y := m.project(mk.lat, mk.lon, w, h)
			if home != nil {
				dx, dy := x-hx, y-hy
				dist := float32(math.Hypot(float64(dx), float64(dy)))
				if dist > 4 {
					cx, cy := (hx+x)/2, (hy+y)/2-dist*0.28
					arc := p.Path()
					arc.MoveTo(hx, hy)
					arc.QuadTo(cx, cy, x, y)
					arc.Stroke(1.4, giode.Mix(mk.color, "theme.surface", 0.55))
				}
			}
		}
		for _, mk := range markers {
			x, y := m.project(mk.lat, mk.lon, w, h)
			r := 3 + float32(mk.weight)*7
			p.Circle(x, y, r*2, giode.Mix(mk.color, "theme.surface", 0.18))
			p.Circle(x, y, r, mk.color)
			p.Circle(x, y, max(r*0.4, 1.5), "#ffffff")
		}
		if home != nil {
			p.Circle(hx, hy, 12, giode.Mix("theme.success", "theme.surface", 0.25))
			p.Circle(hx, hy, 6, "theme.success")
			p.Circle(hx, hy, 2.5, "#ffffff")
		}
	})
	return giode.Box(giode.Styles{Width: -1, Position: giode.PositionRelative},
		land,
		giode.Box(giode.Styles{Position: giode.PositionAbsolute, Left: giode.Flush, Top: giode.Flush, Right: giode.Flush}, overlay),
	)
}

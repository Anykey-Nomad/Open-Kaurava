// Package screenui parses data-only Lua tables used by Relic .screen files.
// It retains unknown properties and widget types for incremental interpretation.
package screenui

import (
	"fmt"
	"path"
	"strings"

	glua "github.com/yuin/gopher-lua"
)

// Vec2 is a normalized position or size in the screen coordinate space.
type Vec2 [2]float64

// Widget is a retained node in a screen's authored widget tree.
type Widget struct {
	Type        string
	Name        string
	ID          int
	Text        string
	Visible     *bool
	Position    Vec2
	Size        Vec2
	HasPosition bool
	HasSize     bool
	Texture     string
	// States — состояния, для которых описан этот арт (Normal, Active,
	// Hover, Disabled). У кнопки они задают плитку кнопки, у остальных
	// виджетов список пуст.
	States []string
	// IsArt — этот виджет пришёл из Presentation.Art владельца: он уже
	// входит в TextureRects владельца и отдельно не обходится.
	IsArt bool
	Art   *Widget
	// aspect — авторский аспект раскладки (.screen AspectRatio). По нему
	// считается ужатие HUD на экране шире авторского (см. uiScale).
	aspect float64
	// anchorX/anchorY — край, за который блок держится при ужатии: −1
	// левый/верхний, +1 правый/нижний, 0 — не определён (по центру).
	anchorX    int8
	anchorY    int8
	ArtNodes   []*Widget
	Owner      *Widget
	Parent     *Widget
	Children   []*Widget
	Properties map[string]glua.LValue
}

// Screen is the parsed root and authored aspect ratio of a .screen file.
type Screen struct {
	Version     int
	AspectRatio float64
	Widgets     *Widget
	Tooltip     *Widget
	byName      map[string]*Widget
}

// Rect is a widget's resolved rectangle in pixels.
type Rect struct{ X, Y, W, H float32 }

// Image is one resolved UI quad: the texture path and where it lands.
type Image struct {
	Path string
	Rect Rect
	ID   int
}

// Parse parses a data-only Lua chunk with all standard libraries disabled.
func Parse(data []byte) (*Screen, error) {
	vm := glua.NewState(glua.Options{SkipOpenLibs: true})
	defer vm.Close()
	if err := vm.DoString(string(data)); err != nil {
		return nil, fmt.Errorf("parse screen Lua: %w", err)
	}
	root, ok := vm.GetGlobal("Screen").(*glua.LTable)
	if !ok {
		return nil, fmt.Errorf("screen has no Screen table")
	}
	s := &Screen{
		Version:     int(number(root.RawGetString("version"))),
		AspectRatio: number(root.RawGetString("AspectRatio")),
		Widgets:     parseWidget(root.RawGetString("Widgets")),
		Tooltip:     parseWidget(root.RawGetString("TooltipWidgets")),
	}
	if s.Widgets == nil {
		return nil, fmt.Errorf("screen has no Widgets table")
	}
	s.byName = make(map[string]*Widget)
	indexWidgets(s.Widgets, s.byName)
	indexWidgets(s.Tooltip, s.byName)
	s.applyWidgetInfo(vm.GetGlobal("ToolInfo"))
	s.inheritButtonGeometry()
	s.propagateAspect()
	s.resolveAnchors()
	return s, nil
}

// resolveAnchors раздаёт виджетам край, за который они держатся при ужатии
// HUD. Край определяет собственный арт блока: панель консоли выходит за
// правый край экрана, панель миникарты — за левый. Внутренние группы
// наследуют край блока, поэтому панель выделения едет вместе с консолью, а
// не остаётся на своём месте относительно экрана.
func (s *Screen) resolveAnchors() {
	if s.Widgets == nil {
		return
	}
	var walk func(*Widget, int8, int8)
	walk = func(w *Widget, ax, ay int8) {
		if ownX, ownY, own := w.artAnchor(); own {
			ax, ay = ownX, ownY
		}
		w.anchorX, w.anchorY = ax, ay
		for _, child := range w.Children {
			walk(child, ax, ay)
		}
	}
	walk(s.Widgets, 0, 0)
}

// artAnchor — край, за который держится сам виджет по своему арту. Третий
// результат — арт найден: без него край наследуется от блока.
func (w *Widget) artAnchor() (int8, int8, bool) {
	// У арт-виджета край задаёт его собственная рамка.
	if w.IsArtLabel() && w.HasSize {
		nx, ny := w.screenOrigin()
		ax, ay := edgeAnchor(nx, ny, w.Size[0], w.Size[1])
		return ax, ay, true
	}
	// У группы — арт её блоков. Он решает край, только если выходит за один
	// бок: у общего фона с левой и правой панелью бока разные, и тогда
	// край не задаётся — его решают сами блоки.
	var overflowLeft, overflowRight, overflowTop, overflowBottom bool
	found := false
	collect := func(nx, ny, sizeX, sizeY float64) {
		if sizeX <= 0 || sizeY <= 0 {
			return
		}
		found = true
		if nx < 0 {
			overflowLeft = true
		}
		if nx+sizeX > 1 {
			overflowRight = true
		}
		if ny < 0 {
			overflowTop = true
		}
		if ny+sizeY > 1 {
			overflowBottom = true
		}
	}
	for _, art := range w.ArtNodes {
		if art == nil || !art.HasSize {
			continue
		}
		nx, ny := art.screenOrigin()
		collect(nx, ny, art.Size[0], art.Size[1])
	}
	for _, child := range w.Children {
		// Арт-виджеты тоже считаются: у панелей artTaskbarRight — отдельный
		// ArtLabel, и его рамка и есть арт блока.
		if child == nil || !child.HasSize || !child.IsArtLabel() || !child.IsVisible() {
			continue
		}
		nx, ny := child.screenOrigin()
		collect(nx, ny, child.Size[0], child.Size[1])
	}
	if !found {
		return 0, 0, false
	}
	var ax, ay int8
	if overflowLeft != overflowRight {
		if overflowLeft {
			ax = -1
		} else {
			ax = 1
		}
	}
	if overflowTop != overflowBottom {
		if overflowTop {
			ay = -1
		} else {
			ay = 1
		}
	}
	return ax, ay, true
}

// Anchor сообщает край, за который виджет держится при ужатии HUD: −1 —
// левый/верхний, +1 — правый/нижний, 0 — по центру.
func (w *Widget) Anchor() (int8, int8) {
	if w == nil {
		return 0, 0
	}
	return w.anchorX, w.anchorY
}

// edgeAnchor — край, за который выходит рамка: только левый, только правый и
// так далее. Рамка, перекрывшая оба края, край не задаёт.
func edgeAnchor(nx, ny, sizeX, sizeY float64) (int8, int8) {
	var ax, ay int8
	left, right := nx < 0, nx+sizeX > 1
	if left != right {
		if left {
			ax = -1
		} else {
			ax = 1
		}
	}
	top, bottom := ny < 0, ny+sizeY > 1
	if top != bottom {
		if top {
			ay = -1
		} else {
			ay = 1
		}
	}
	return ax, ay
}

// propagateAspect раздаёт виджетам авторский аспект раскладки: Layout считает
// по нему ужатие HUD на широком экране.
func (s *Screen) propagateAspect() {
	var walk func(*Widget)
	walk = func(w *Widget) {
		if w == nil {
			return
		}
		w.aspect = s.AspectRatio
		for _, child := range w.Children {
			walk(child)
		}
	}
	walk(s.Widgets)
}

// inheritButtonGeometry даёт кнопкам без своего size геометрию соседней
// кнопки: в .screen часть кнопок верхней полосы описана через шаблон и не
// хранит size (у btnDIP его нет, у btnOBJ есть). Без размера арт состояний
// считается от вьюпорта, и menu_button_1 ложится плитой на весь экран.
// Позиция у такой кнопки своя, поэтому копируется только размер.
func (s *Screen) inheritButtonGeometry() {
	if s == nil || s.Widgets == nil {
		return
	}
	var walk func(*Widget)
	walk = func(w *Widget) {
		for _, child := range w.Children {
			if child.IsButton() && !child.HasSize {
				for _, sibling := range w.Children {
					if sibling != child && sibling.IsButton() && sibling.HasSize {
						child.Size, child.HasSize = sibling.Size, true
						break
					}
				}
			}
			walk(child)
		}
	}
	walk(s.Widgets)
}

// applyWidgetInfo применяет экземплярные переопределения из секции
// ToolInfo.WidgetInfo (.screen): она задаёт состояние виджетов по именам.
// В разнесённой раскладке ремастера там спрятана сплошная полоса taskbar
// (artTaskbar/artTaskbarPadded), а миникарта и консоль остаются видимы.
func (s *Screen) applyWidgetInfo(value glua.LValue) {
	table, ok := value.(*glua.LTable)
	if !ok {
		return
	}
	info, ok := table.RawGetString("WidgetInfo").(*glua.LTable)
	if !ok {
		return
	}
	var walk func(glua.LValue)
	walk = func(v glua.LValue) {
		tbl, ok := v.(*glua.LTable)
		if !ok {
			return
		}
		if name, ok := tbl.RawGetString("name").(glua.LString); ok && name != "" {
			if hidden, ok := tbl.RawGetString("hidden").(glua.LBool); ok {
				if w := s.Find(string(name)); w != nil {
					visible := !bool(hidden)
					w.Visible = &visible
				}
			}
		}
		tbl.ForEach(func(_, child glua.LValue) {
			walk(child)
		})
	}
	walk(info)
}

func indexWidgets(w *Widget, index map[string]*Widget) {
	if w == nil {
		return
	}
	if w.Name != "" {
		if _, exists := index[strings.ToLower(w.Name)]; !exists {
			index[strings.ToLower(w.Name)] = w
		}
	}
	for _, child := range w.Children {
		indexWidgets(child, index)
	}
}

func parseWidget(value glua.LValue) *Widget {
	table, ok := value.(*glua.LTable)
	if !ok {
		return nil
	}
	w := &Widget{Properties: make(map[string]glua.LValue)}
	var artNodes []*Widget
	if presentation, ok := table.RawGetString("Presentation").(*glua.LTable); ok {
		if presentation.RawGetString("Art") != glua.LNil {
			w.Properties["Presentation.Art"] = presentation.RawGetString("Art")
			artNodes = parseArtNodes(presentation.RawGetString("Art"))
			w.ArtNodes = artNodes
			if len(artNodes) > 0 {
				w.Art = artNodes[0]
				w.Art.Parent = w
			}
		}
		if presentation.RawGetString("Text") != glua.LNil {
			w.Properties["Presentation.Text"] = presentation.RawGetString("Text")
		}
	}
	if v, ok := table.RawGetString("type").(glua.LString); ok {
		w.Type = string(v)
	}
	if v, ok := table.RawGetString("name").(glua.LString); ok {
		w.Name = string(v)
	}
	if v, ok := table.RawGetString("text").(glua.LString); ok {
		w.Text = string(v)
	}
	if w.Text == "" {
		if v, ok := table.RawGetString("Text").(glua.LString); ok {
			w.Text = string(v)
		}
	}
	if art, ok := table.RawGetString("Presentation").(*glua.LTable); ok {
		if label, ok := art.RawGetString("Text").(*glua.LTable); ok {
			if text, ok := label.RawGetString("text").(glua.LString); ok && w.Text == "" {
				w.Text = string(text)
			}
		}
	}
	if v, ok := table.RawGetString("texture").(glua.LString); ok {
		w.Texture = string(v)
	}
	if states, ok := table.RawGetString("states").(*glua.LTable); ok {
		for i := 1; ; i++ {
			value := states.RawGetInt(i)
			if value == glua.LNil {
				break
			}
			if state, ok := value.(glua.LString); ok {
				w.States = append(w.States, string(state))
			}
		}
	}
	if v, ok := table.RawGetString("ID").(glua.LNumber); ok {
		w.ID = int(v)
	}
	w.Visible = parseVisibility(table)
	w.Position, w.HasPosition = vector2(table.RawGetString("position"))
	w.Size, w.HasSize = vector2(table.RawGetString("size"))
	table.ForEach(func(key, value glua.LValue) {
		if name, ok := key.(glua.LString); ok {
			w.Properties[string(name)] = value
		}
	})
	appendChildren(w, table.RawGetString("Children"))
	for _, art := range artNodes {
		if art == nil {
			continue
		}
		art.Parent = w
		art.Owner = w
		art.IsArt = true
		w.Children = append(w.Children, art)
	}
	if presentation, ok := table.RawGetString("Presentation").(*glua.LTable); ok {
		appendChildren(w, presentation.RawGetString("Text"))
	}
	return w
}

func parseArtNodes(value glua.LValue) []*Widget {
	table, ok := value.(*glua.LTable)
	if !ok {
		return nil
	}
	if table.RawGetString("type") != glua.LNil {
		return []*Widget{parseWidget(table)}
	}
	var nodes []*Widget
	for i := 1; ; i++ {
		child := table.RawGetInt(i)
		if child == glua.LNil {
			break
		}
		if parsed := parseWidget(child); parsed != nil {
			nodes = append(nodes, parsed)
		}
	}
	return nodes
}

func appendChildren(parent *Widget, value glua.LValue) {
	children, ok := value.(*glua.LTable)
	if !ok {
		return
	}
	children.ForEach(func(_, child glua.LValue) {
		if parsed := parseWidget(child); parsed != nil {
			parsed.Parent = parent
			parsed.Owner = parent
			parent.Children = append(parent.Children, parsed)
		}
	})
}

func parseVisibility(table *glua.LTable) *bool {
	if hidden, ok := table.RawGetString("hidden").(glua.LBool); ok {
		visible := !bool(hidden)
		return &visible
	}
	if authored, ok := table.RawGetString("visible").(glua.LBool); ok {
		visible := bool(authored)
		return &visible
	}
	return nil
}

func vector2(value glua.LValue) (Vec2, bool) {
	table, ok := value.(*glua.LTable)
	if !ok {
		return Vec2{}, false
	}
	return Vec2{number(table.RawGetInt(1)), number(table.RawGetInt(2))}, true
}

func number(value glua.LValue) float64 {
	n, _ := value.(glua.LNumber)
	return float64(n)
}

// Find returns the first widget with the exact name, depth-first.
func (w *Widget) Find(name string) *Widget {
	if w == nil {
		return nil
	}
	if strings.EqualFold(w.Name, name) {
		return w
	}
	for _, child := range w.Children {
		if found := child.Find(name); found != nil {
			return found
		}
	}
	return nil
}

// Find returns the first matching widget by name, case-insensitively.
func (s *Screen) Find(name string) *Widget {
	if s == nil || s.byName == nil {
		return nil
	}
	return s.byName[strings.ToLower(name)]
}

// IsVisible returns initial visibility inherited through the parent chain.
// Missing visibility means visible; runtime bindings can override it later.
func (w *Widget) IsVisible() bool {
	for current := w; current != nil; current = current.Parent {
		if current.Visible != nil && !*current.Visible {
			return false
		}
	}
	return w != nil
}

// Layout resolves a named widget through its complete parent chain.
func (s *Screen) Layout(name string, viewport Rect) (Rect, bool) {
	w := s.Find(name)
	if w == nil {
		return Rect{}, false
	}
	return w.Layout(viewport), true
}

// screenOrigin — начало виджета в нормализованных координатах экрана:
// позиции всех предков складываются, как их считает движок.
func (w *Widget) screenOrigin() (float64, float64) {
	var nx, ny float64
	for current := w; current != nil; current = current.Parent {
		if current.HasPosition {
			nx += current.Position[0]
			ny += current.Position[1]
		}
	}
	return nx, ny
}

// IsArtLabel — виджет-арт (.screen рисует такие прямо из файла).
func (w *Widget) IsArtLabel() bool {
	return w != nil && strings.EqualFold(w.Type, "ArtLabel")
}

// Layout resolves .screen geometry. Relic position and size values are
// normalized to the full viewport; parent offsets are accumulated but do not
// scale child coordinates. Some authored groups intentionally extend beyond
// their parent's bounds before being clipped by the viewport.
func (w *Widget) TexturePath() string {
	if w == nil {
		return ""
	}
	if w.Texture != "" {
		return cleanTexturePath(w.Texture)
	}
	return ""
}

func cleanTexturePath(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	// В .screen пути идут со схемой ("generic:art/ui/..."): схема — это
	// ссылка на общий набор ресурсов, в дереве модуля лежит сам путь.
	if colon := strings.IndexByte(value, ':'); colon > 0 {
		scheme := value[:colon]
		valid := true
		for _, r := range scheme {
			if r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
				valid = false
				break
			}
		}
		if valid {
			value = value[colon+1:]
		}
	}
	value = strings.TrimLeft(value, "/")
	if strings.HasPrefix(strings.ToLower(value), "data/") {
		value = value[len("data/"):]
	}
	return path.Clean(value)
}

// ButtonType — тип виджета-кнопки в .screen. Арт кнопки описан состояниями,
// а координаты Graphic заданы долями прямоугольника кнопки из стиля.
const ButtonType = "Button"

// artStatesOrder — в каком порядке выбирается состояние кнопки. Рисуется
// ровно один арт состояния: иначе Normal, Active, Hover и Disabled ложатся
// друг на друга.
var artStatesOrder = []string{"Normal", "Active", "Hover", "Disabled"}

// TextureRects resolves the UI image quads carried by this widget's art tree.
// Geometry is screen-normalized like the owning widget, while each Graphic
// applies its own position and size within the owner's bounds.
//
// У кнопки арт состояния ложится в собственный прямоугольник кнопки и берётся
// только одно состояние: в .screen позиция и размер таких Graphic — доли
// кнопки, а не экрана.
func (w *Widget) TextureRects(viewport Rect) []Image {
	if w == nil {
		return nil
	}
	ownerRect := w.Layout(viewport)
	state := ""
	if w.IsButton() {
		state = w.artState()
	}
	var out []Image
	var walk func(*Widget, Rect)
	walk = func(node *Widget, parent Rect) {
		if node == nil {
			return
		}
		if node != w && !node.IsVisible() {
			return
		}
		stateArt := state != "" && node.Owner == w && len(node.States) > 0
		if stateArt && !node.HasState(state) {
			return
		}
		owner := parent
		if node.Owner != nil {
			owner = node.Owner.Layout(viewport)
		}
		r := owner
		if stateArt {
			r = ownerRect
		} else {
			if node.HasPosition {
				r.X += float32(node.Position[0]) * owner.W
				r.Y += float32(node.Position[1]) * owner.H
			}
			if node.HasSize {
				r.W = float32(node.Size[0]) * owner.W
				r.H = float32(node.Size[1]) * owner.H
			}
		}
		if texture := node.TexturePath(); texture != "" && r.W > 0 && r.H > 0 {
			out = append(out, Image{Path: texture, Rect: r, ID: node.ID})
		}
		for _, child := range node.Children {
			walk(child, r)
		}
	}
	for _, art := range w.ArtNodes {
		walk(art, ownerRect)
	}
	if texture := w.TexturePath(); texture != "" {
		return append([]Image{{Path: texture, Rect: ownerRect, ID: w.ID}}, out...)
	}
	return out
}

// IsButton сообщает, что виджет — кнопка со стилевым артом состояний.
func (w *Widget) IsButton() bool {
	return w != nil && strings.EqualFold(w.Type, ButtonType)
}

// artState — состояние, арт которого рисуется у кнопки: первое из
// artStatesOrder, которое вообще описано в её ArtNodes.
func (w *Widget) artState() string {
	for _, state := range artStatesOrder {
		for _, art := range w.ArtNodes {
			if art.HasState(state) {
				return state
			}
		}
	}
	return ""
}

// HasState сообщает, описан ли этот арт для состояния state.
func (w *Widget) HasState(state string) bool {
	for _, s := range w.States {
		if strings.EqualFold(s, state) {
			return true
		}
	}
	return false
}

// uiScale — во сколько раз движок ужимает раскладку .screen на экране шире
// авторского. Раскладка нарисована под AspectRatio (4:3), а ремастер на 16:9
// держит блоки того же физического размера: они ужимаются на отношение
// аспектов (4:3 → 16:9 даёт 0.75) и прижимаются к ближайшему краю. Без
// ужатия панели растягиваются по ширине и становятся выше, чем в игре.
func (w *Widget) uiScale(viewport Rect) float32 {
	if w == nil {
		return 1
	}
	return scaleFor(w.aspect, viewport)
}

// scaleFor — коэффициент ужатия раскладки: отношение авторского аспекта
// к аспекту окна, не больше 1.
func scaleFor(aspect float64, viewport Rect) float32 {
	if aspect <= 0 || viewport.W <= 0 || viewport.H <= 0 {
		return 1
	}
	scale := aspect / (float64(viewport.W) / float64(viewport.H))
	if scale >= 1 {
		return 1
	}
	return float32(scale)
}

// Scale сообщает, во сколько раз ужата раскладка на этом окне. Сцены ужимают
// им шрифты и отступы внутри панелей: они заданы в пикселях и без этого
// остаются крупными для ужатых клеток.
func (s *Screen) Scale(viewport Rect) float32 {
	if s == nil {
		return 1
	}
	return scaleFor(s.AspectRatio, viewport)
}

func (w *Widget) Layout(viewport Rect) Rect {
	if w == nil {
		return viewport
	}
	var nx, ny float64
	for current := w; current != nil; current = current.Parent {
		if current.HasPosition {
			nx += current.Position[0]
			ny += current.Position[1]
		}
	}
	if !w.HasSize {
		// Контейнер без своего размера занимает вьюпорт целиком: его
		// содержимое считается от экрана, а не от него.
		return Rect{X: float32(nx) * viewport.W, Y: float32(ny) * viewport.H, W: viewport.W, H: viewport.H}
	}
	x := float32(nx) * viewport.W
	y := float32(ny) * viewport.H
	width := float32(w.Size[0]) * viewport.W
	height := float32(w.Size[1]) * viewport.H
	if scale := w.uiScale(viewport); scale < 1 {
		// Прижатие к краю: блок держится за тот край, за который выходит
		// его арт, а внутренние группы — за край своего блока (см.
		// resolveAnchors). Без подсказки — по центру.
		switch {
		case w.anchorX > 0:
			x = viewport.W - (viewport.W-x)*scale
		case w.anchorX < 0:
			x *= scale
		case nx+w.Size[0]/2 >= 0.5:
			x = viewport.W - (viewport.W-x)*scale
		default:
			x *= scale
		}
		switch {
		case w.anchorY > 0:
			y = viewport.H - (viewport.H-y)*scale
		case w.anchorY < 0:
			y *= scale
		case ny+w.Size[1]/2 >= 0.5:
			y = viewport.H - (viewport.H-y)*scale
		default:
			y *= scale
		}
		width *= scale
		height *= scale
	}
	return Rect{X: x, Y: y, W: width, H: height}
}

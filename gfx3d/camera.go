// Package gfx3d — 3D-рендерер сцены сценария на чистом Vulkan без CGO:
// орбитальная камера, матрицы вида и проекции, загрузка текстур и мешей,
// пайплайн отрисовки рельефа, объектов и воды.
package gfx3d

import "math"

// Начальные параметры камеры (как в 3D-просмотрщике сценариев dow-mod-studio).
const (
	DefaultDistance = 70.0
	MinDistance     = 10.0
	DefaultPitch    = 49.481994013136 * math.Pi / 180
	DefaultYaw      = -40.205715405573 * math.Pi / 180
	DefaultFOV      = 35.0
)

// Camera — орбитальная камера вокруг точки Target в мире Z-up.
type Camera struct {
	Yaw, Pitch       float64
	Distance         float64
	MinDist, MaxDist float64
	Target           [3]float64
	FOV              float64
	GroundMin        [2]float64
	GroundMax        [2]float64
	CellSize         float64
	GroundHeight     func(x, y float64) float64
}

// NewCamera создаёт камеру с настройками по умолчанию.
func NewCamera() Camera {
	return Camera{
		Yaw:      DefaultYaw,
		Pitch:    DefaultPitch,
		Distance: DefaultDistance,
		MinDist:  MinDistance,
		MaxDist:  800.0,
		FOV:      DefaultFOV,
		CellSize: 2.0,
	}
}

// FitToScene настраивает камеру под границы сцены сценария.
//
// Расстояние считается так, чтобы карта целиком влезла в кадр: карта
// вписывается в шар радиусом половины диагонали, а шар виден целиком
// с расстояния radius / sin(fov/2).
func (c *Camera) FitToScene(groundMin, groundMax [2]float32, centerZ float32, cellSize float32) {
	c.GroundMin = [2]float64{float64(groundMin[0]), float64(groundMin[1])}
	c.GroundMax = [2]float64{float64(groundMax[0]), float64(groundMax[1])}
	if cellSize > 0 {
		c.CellSize = float64(cellSize)
	}
	cx := (c.GroundMin[0] + c.GroundMax[0]) * 0.5
	cy := (c.GroundMin[1] + c.GroundMax[1]) * 0.5
	c.Target = [3]float64{cx, cy, float64(centerZ)}
	c.UpdateTargetHeight()

	w := c.GroundMax[0] - c.GroundMin[0]
	d := c.GroundMax[1] - c.GroundMin[1]
	radius := math.Hypot(w, d) / 2
	fov := c.FOV
	if fov <= 0 {
		fov = DefaultFOV
	}
	dist := radius / math.Sin(fov*math.Pi/360)
	if dist < MinDistance {
		dist = MinDistance
	}
	c.Distance = dist * 1.15
	c.MinDist = math.Max(MinDistance, c.CellSize*4)
	c.MaxDist = math.Max(c.Distance*2, c.MinDist*2)
	c.Yaw = DefaultYaw
	c.Pitch = DefaultPitch
}

// Orbit поворачивает камеру на dx, dy пикселей.
func (c *Camera) Orbit(dx, dy float64) {
	c.Yaw -= dx * 0.01
	c.Pitch += dy * 0.01
	if c.Pitch > 1.45 {
		c.Pitch = 1.45
	}
	if c.Pitch < 0.05 {
		c.Pitch = 0.05
	}
}

// Pan сдвигает точку прицеливания камеры вдоль экранных осей.
func (c *Camera) Pan(dx, dy float64, viewHeight int) {
	if viewHeight <= 0 {
		return
	}
	fov := c.FOV
	if fov <= 0 {
		fov = DefaultFOV
	}
	scale := 2 * c.Distance * math.Tan(fov*math.Pi/360) / float64(viewHeight)
	forward, right := c.axes()
	// В плоскости XY: сдвиг мышью вправо тянет землю вправо (точка уходит влево).
	c.Target[0] -= (right[0]*dx - forward[0]*dy) * scale
	c.Target[1] -= (right[1]*dx - forward[1]*dy) * scale
	c.clampTarget()
}

// Move перемещает камеру по карте (forward — вперёд/назад, strafe — влево/вправо).
func (c *Camera) Move(forward, strafe, dt float64) {
	if dt <= 0 || (forward == 0 && strafe == 0) {
		return
	}
	length := math.Hypot(forward, strafe)
	if length > 1 {
		forward /= length
		strafe /= length
	}
	speed := math.Max(c.CellSize*12, 35.0)
	fx, fy := -math.Cos(c.Yaw), -math.Sin(c.Yaw)
	// Вектор «вправо» — поворот направления взгляда на 90° по часовой стрелке:
	// при yaw = 0 камера смотрит вдоль -X, а вправо от неё — +Y. Обратный
	// поворот давал вектор влево, из-за чего A и D работали наоборот.
	rx, ry := fy, -fx
	dist := speed * dt
	c.Target[0] += (fx*forward + rx*strafe) * dist
	c.Target[1] += (fy*forward + ry*strafe) * dist
	c.clampTarget()
}

// Zoom приближает или отдаляет камеру по прокрутке колеса.
func (c *Camera) Zoom(scroll float64) {
	if scroll == 0 {
		return
	}
	factor := math.Pow(0.88, scroll)
	c.Distance *= factor
	if c.Distance < c.MinDist {
		c.Distance = c.MinDist
	}
	if c.MaxDist > c.MinDist && c.Distance > c.MaxDist {
		c.Distance = c.MaxDist
	}
}

// SetGroundHeight supplies the sampled terrain height at a world XY position.
func (c *Camera) SetGroundHeight(sample func(x, y float64) float64) {
	c.GroundHeight = sample
}

// UpdateTargetHeight keeps the orbit pivot on the ground surface.
func (c *Camera) UpdateTargetHeight() {
	if c.GroundHeight != nil {
		c.Target[2] = c.GroundHeight(c.Target[0], c.Target[1])
	}
}

func (c *Camera) clampTarget() {
	if c.GroundMax[0] > c.GroundMin[0] {
		c.Target[0] = math.Max(c.GroundMin[0], math.Min(c.GroundMax[0], c.Target[0]))
	}
	if c.GroundMax[1] > c.GroundMin[1] {
		c.Target[1] = math.Max(c.GroundMin[1], math.Min(c.GroundMax[1], c.Target[1]))
	}
	c.UpdateTargetHeight()
}

// Eye — положение камеры в мире: точка цели плюс направление взгляда на
// расстояние орбиты. Нужна эффектам: квады частиц разворачиваются к ней.
func (c Camera) Eye() [3]float64 {
	pitch := c.Pitch
	if pitch > 1.5 {
		pitch = 1.5
	}
	if pitch < -1.5 {
		pitch = -1.5
	}
	dist := c.Distance
	if dist <= 0 {
		dist = 1
	}
	dir := [3]float64{
		math.Cos(c.Yaw) * math.Cos(pitch),
		math.Sin(c.Yaw) * math.Cos(pitch),
		math.Sin(pitch),
	}
	return [3]float64{
		c.Target[0] + dir[0]*dist,
		c.Target[1] + dir[1]*dist,
		c.Target[2] + dir[2]*dist,
	}
}

func (c *Camera) axes() (forward, right [2]float64) {
	fx, fy := -math.Cos(c.Yaw), -math.Sin(c.Yaw)
	return [2]float64{fx, fy}, [2]float64{-fy, fx}
}

// ViewProj строит column-major матрицу ViewProj (16 float32) для Vulkan:
// Z в диапазоне [0, 1] и перевёрнутая ось Y клип-пространства.
// RayFromScreen unprojects a top-left-origin pixel into a world-space ray.
func (c *Camera) RayFromScreen(x, y, width, height int) (origin, direction [3]float64, ok bool) {
	if width <= 0 || height <= 0 {
		return origin, direction, false
	}
	inverse, ok := inverseMat4(c.ViewProj(width, height))
	if !ok {
		return origin, direction, false
	}
	nx := 2*float64(x)/float64(width) - 1
	ny := 2*float64(y)/float64(height) - 1
	near, okNear := unproject(inverse, [4]float64{nx, ny, 0, 1})
	far, okFar := unproject(inverse, [4]float64{nx, ny, 1, 1})
	if !okNear || !okFar {
		return origin, direction, false
	}
	origin = [3]float64{near[0], near[1], near[2]}
	direction = normalize3([3]float64{far[0] - near[0], far[1] - near[1], far[2] - near[2]})
	return origin, direction, direction != ([3]float64{})
}

func inverseMat4(matrix [16]float32) ([16]float64, bool) {
	var rows [4][8]float64
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			rows[row][col] = float64(matrix[col*4+row])
		}
		rows[row][row+4] = 1
	}
	for col := 0; col < 4; col++ {
		pivot := col
		for row := col + 1; row < 4; row++ {
			if math.Abs(rows[row][col]) > math.Abs(rows[pivot][col]) {
				pivot = row
			}
		}
		if math.Abs(rows[pivot][col]) < 1e-12 {
			return [16]float64{}, false
		}
		rows[col], rows[pivot] = rows[pivot], rows[col]
		divisor := rows[col][col]
		for j := 0; j < 8; j++ {
			rows[col][j] /= divisor
		}
		for row := 0; row < 4; row++ {
			if row == col {
				continue
			}
			factor := rows[row][col]
			for j := 0; j < 8; j++ {
				rows[row][j] -= factor * rows[col][j]
			}
		}
	}
	var inverse [16]float64
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			inverse[col*4+row] = rows[row][col+4]
		}
	}
	return inverse, true
}

func unproject(inverse [16]float64, point [4]float64) ([3]float64, bool) {
	var world [4]float64
	for row := 0; row < 4; row++ {
		for col := 0; col < 4; col++ {
			world[row] += inverse[col*4+row] * point[col]
		}
	}
	if math.Abs(world[3]) < 1e-12 {
		return [3]float64{}, false
	}
	return [3]float64{world[0] / world[3], world[1] / world[3], world[2] / world[3]}, true
}

func (c *Camera) ViewProj(width, height int) [16]float32 {
	if width <= 0 {
		width = 1
	}
	if height <= 0 {
		height = 1
	}
	pitch := c.Pitch
	if pitch > 1.5 {
		pitch = 1.5
	}
	if pitch < -1.5 {
		pitch = -1.5
	}
	dist := c.Distance
	if dist <= 0 {
		dist = 1
	}
	dir := [3]float64{
		math.Cos(c.Yaw) * math.Cos(pitch),
		math.Sin(c.Yaw) * math.Cos(pitch),
		math.Sin(pitch),
	}
	eye := [3]float64{
		c.Target[0] + dir[0]*dist,
		c.Target[1] + dir[1]*dist,
		c.Target[2] + dir[2]*dist,
	}
	view := lookAt(eye, c.Target, [3]float64{0, 0, 1})
	fov := c.FOV
	if fov <= 0 {
		fov = DefaultFOV
	}
	near := math.Max(0.2, dist/500)
	far := dist*10 + 1500
	proj := perspectiveVulkan(fov*math.Pi/180, float64(width)/float64(height), near, far)
	return mulMat4(proj, view)
}

// Project переводит точку мира в пиксели кадра. ok=false — точка за
// камерой или вне глубины кадра (её всё равно не видно).
func (c *Camera) Project(point [3]float64, width, height int) (x, y float64, ok bool) {
	vp := c.ViewProj(width, height)
	clip := [4]float64{}
	for row := 0; row < 4; row++ {
		clip[row] = float64(vp[row])*point[0] + float64(vp[4+row])*point[1] +
			float64(vp[8+row])*point[2] + float64(vp[12+row])
	}
	if clip[3] <= 1e-9 {
		return 0, 0, false
	}
	ndcX, ndcY, ndcZ := clip[0]/clip[3], clip[1]/clip[3], clip[2]/clip[3]
	if ndcZ < 0 || ndcZ > 1 {
		return 0, 0, false
	}
	x = (ndcX*0.5 + 0.5) * float64(width)
	// Вертикаль кадра и NDC совпадают по направлению (Y вниз).
	y = (ndcY*0.5 + 0.5) * float64(height)
	return x, y, true
}

// lookAt возвращает column-major матрицу вида 4x4.
func lookAt(eye, center, up [3]float64) [16]float64 {
	f := normalize3([3]float64{center[0] - eye[0], center[1] - eye[1], center[2] - eye[2]})
	s := normalize3(cross3(f, up))
	u := cross3(s, f)

	// Column-major: m[col*4 + row]
	return [16]float64{
		s[0], u[0], -f[0], 0,
		s[1], u[1], -f[1], 0,
		s[2], u[2], -f[2], 0,
		-dot3(s, eye), -dot3(u, eye), dot3(f, eye), 1,
	}
}

// perspectiveVulkan строит column-major матрицу перспективы для Vulkan:
// ось Y направлена вниз (m[5] = -f), глубина Z в [0, 1].
func perspectiveVulkan(fovyRad, aspect, near, far float64) [16]float64 {
	f := 1.0 / math.Tan(fovyRad/2)
	if aspect <= 0 {
		aspect = 1
	}
	nf := near - far
	if math.Abs(nf) < 1e-6 {
		nf = -1
	}
	return [16]float64{
		f / aspect, 0, 0, 0,
		0, -f, 0, 0,
		0, 0, far / nf, -1,
		0, 0, (near * far) / nf, 0,
	}
}

func mulMat4(a, b [16]float64) [16]float32 {
	var out [16]float32
	for col := 0; col < 4; col++ {
		for row := 0; row < 4; row++ {
			var sum float64
			for k := 0; k < 4; k++ {
				sum += a[k*4+row] * b[col*4+k]
			}
			out[col*4+row] = float32(sum)
		}
	}
	return out
}

func cross3(a, b [3]float64) [3]float64 {
	return [3]float64{
		a[1]*b[2] - a[2]*b[1],
		a[2]*b[0] - a[0]*b[2],
		a[0]*b[1] - a[1]*b[0],
	}
}

func dot3(a, b [3]float64) float64 {
	return a[0]*b[0] + a[1]*b[1] + a[2]*b[2]
}

func normalize3(v [3]float64) [3]float64 {
	n := math.Sqrt(v[0]*v[0] + v[1]*v[1] + v[2]*v[2])
	if n < 1e-9 {
		return [3]float64{}
	}
	return [3]float64{v[0] / n, v[1] / n, v[2] / n}
}

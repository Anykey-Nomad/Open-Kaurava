// Package shaders — встроенные в бинарник SPIR-V модули UI и 3D пайплайнов.
//
// Исходники GLSL лежат рядом; .spv собирается ими:
//
//	glslc --target-env=vulkan1.0 -fshader-stage=vert -o quad.vert.spv quad.vert
//	glslc --target-env=vulkan1.0 -fshader-stage=frag -o quad.frag.spv quad.frag
//	glslc --target-env=vulkan1.0 -fshader-stage=vert -o mesh3d.vert.spv mesh3d.vert
//	glslc --target-env=vulkan1.0 -fshader-stage=frag -o mesh3d.frag.spv mesh3d.frag
//
// На машине с Vulkan SDK (glslc в PATH) — пересобрать: go generate ./...
package shaders

import _ "embed"

//go:generate glslc --target-env=vulkan1.0 -fshader-stage=vert -o quad.vert.spv quad.vert
//go:generate glslc --target-env=vulkan1.0 -fshader-stage=frag -o quad.frag.spv quad.frag
//go:generate glslc --target-env=vulkan1.0 -fshader-stage=vert -o mesh3d.vert.spv mesh3d.vert
//go:generate glslc --target-env=vulkan1.0 -fshader-stage=frag -o mesh3d.frag.spv mesh3d.frag

// QuadVert — вершинный шейдер UI: позиция в пикселях → NDC через push constant.
//
//go:embed quad.vert.spv
var QuadVert []byte

// QuadFrag — фрагментный шейдер UI: A8-атлас (R8) × цвет.
//
//go:embed quad.frag.spv
var QuadFrag []byte

// Mesh3DVert — вершинный шейдер 3D-мешей карты и объектов.
//
//go:embed mesh3d.vert.spv
var Mesh3DVert []byte

// Mesh3DFrag — фрагментный шейдер 3D-мешей карты и объектов.
//
//go:embed mesh3d.frag.spv
var Mesh3DFrag []byte

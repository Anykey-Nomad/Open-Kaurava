#version 450

// Вершина UI-квадрата: позиция в пикселях, UV в атласе, цвет.
// Конвертация пикселей → NDC — через push constant (размер экрана),
// поэтому пайплайн не пересоздаётся при resize.
//
// Ось Y в Vulkan направлена ВНИЗ: ndc.y = -1 — верх кадра, ndc.y = +1 — низ
// (в OpenGL наоборот). Экранные пиксели (y вниз, начало в левом верхнем
// углу) поэтому переводятся формулой y/H*2-1, без зеркала: иначе весь
// интерфейс — вместе с курсором — оказывается перевёрнутым.
layout(location = 0) in vec2 inPos;
layout(location = 1) in vec2 inUV;
layout(location = 2) in vec4 inColor;

layout(location = 0) out vec2 outUV;
layout(location = 1) out vec4 outColor;

layout(push_constant) uniform Push {
    vec2 screen; // размер кадра в пикселях
} pc;

void main() {
    vec2 ndc = vec2(inPos.x / pc.screen.x * 2.0 - 1.0,
                    inPos.y / pc.screen.y * 2.0 - 1.0);
    gl_Position = vec4(ndc, 0.0, 1.0);
    outUV = inUV;
    outColor = inColor;
}

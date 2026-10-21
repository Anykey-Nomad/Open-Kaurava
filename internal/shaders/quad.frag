#version 450

// Текстура — A8-атлас глифов (R8_UNORM): в канале R — покрытие пикселя.
// Белый пиксель в углу атласа (UV 0,0) даёт alpha=1 — им рисуются
// непрозрачные прямоугольники UI без отдельного пайплайна.
layout(location = 0) in vec2 inUV;
layout(location = 1) in vec4 inColor;

layout(location = 0) out vec4 outColor;

layout(set = 0, binding = 0) uniform sampler2D tex;

void main() {
    float coverage = texture(tex, inUV).r;
    outColor = vec4(inColor.rgb, inColor.a * coverage);
}

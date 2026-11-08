#version 450

// Частицы (дым, пыль, вспышки): вершины — уже готовые углы квада в мировых
// координатах, поэтому шейдер только проецирует точку и передаёт цвет.
layout(location = 0) in vec3 inPos;
layout(location = 1) in vec2 inUV;
layout(location = 2) in vec4 inColor;

layout(location = 0) out vec2 outUV;
layout(location = 1) out vec4 outColor;

// Раскладка push-констант та же, что у мешей: пайплайн-лейаут общий, и
// шейдер обязан объявить блок не больше его диапазона.
layout(push_constant) uniform Push {
    mat4 viewProj;
    vec4 tint;
    vec4 params;
    ivec4 info;
} pc;

void main() {
    gl_Position = pc.viewProj * vec4(inPos, 1.0);
    outUV = inUV;
    outColor = inColor;
}

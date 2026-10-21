#version 450

// 3D-шейдер мешей карты и объектов (Z вверх в мировых координатах).
// Матрица viewProj передаётся в push-константах с уже отзеркаленной осью Y
// для клип-пространства Vulkan.
layout(location = 0) in vec3 inPos;
layout(location = 1) in vec3 inNormal;
layout(location = 2) in vec2 inUV;

layout(location = 0) out vec3 outNormal;
layout(location = 1) out vec2 outUV;

layout(push_constant) uniform Push {
    mat4 viewProj;
    vec4 tint;   // rgba-модуляция цвета
    vec4 params; // x = alphaRef, y = ambient, z = diffuse, w = unused
} pc;

void main() {
    gl_Position = pc.viewProj * vec4(inPos, 1.0);
    outNormal = inNormal;
    outUV = inUV;
}

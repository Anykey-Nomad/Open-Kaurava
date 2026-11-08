#version 450

// 3D-шейдер мешей карты и объектов (Z вверх в мировых координатах).
// Матрица viewProj передаётся в push-константах с уже отзеркаленной осью Y
// для клип-пространства Vulkan.
//
// Анимируемые объекты скиннятся на GPU: палитра костей кадра лежит в
// storage-буфере (binding 1), локальные индексы костей вершины
// сдвигаются на базу объекта (push-константа info.x). Статичные вершины
// имеют нулевые веса и проходят без преобразований.
layout(location = 0) in vec3 inPos;
layout(location = 1) in vec3 inNormal;
layout(location = 2) in vec2 inUV;
layout(location = 3) in ivec4 inBones;
layout(location = 4) in vec4 inWeights;

layout(location = 0) out vec3 outNormal;
layout(location = 1) out vec2 outUV;

layout(std430, set = 0, binding = 1) readonly buffer Palette {
    mat4 mats[]; // столбиковый порядок, как у GLSL
} palette;

layout(push_constant) uniform Push {
    mat4 viewProj;
    vec4 tint;   // rgba-модуляция цвета
    vec4 params; // x = alphaRef, y = ambient, z = diffuse, w = unused
    ivec4 info;  // x = база костей объекта в палитре
} pc;

void main() {
    vec3 pos = inPos;
    vec3 nrm = inNormal;
    float total = 0.0;
    vec3 skinnedPos = vec3(0.0);
    vec3 skinnedNrm = vec3(0.0);
    for (int i = 0; i < 4; i++) {
        float w = inWeights[i];
        int bone = inBones[i];
        if (w <= 0.0 || bone < 0) {
            continue;
        }
        mat4 m = palette.mats[pc.info.x + bone];
        skinnedPos += w * (m * vec4(inPos, 1.0)).xyz;
        skinnedNrm += w * (mat3(m) * inNormal);
        total += w;
    }
    if (total > 0.0) {
        // Веса нормализуются так же, как на CPU (whm.SkinnedVertex).
        pos = skinnedPos / total;
        nrm = skinnedNrm / total;
    }
    gl_Position = pc.viewProj * vec4(pos, 1.0);
    outNormal = nrm;
    outUV = inUV;
}

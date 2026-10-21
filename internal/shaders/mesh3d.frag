#version 450

// Фрагментный шейдер 3D-мешей: текстура + освещение Ламберта + альфа-тест.
layout(location = 0) in vec3 inNormal;
layout(location = 1) in vec2 inUV;

layout(location = 0) out vec4 outColor;

layout(set = 0, binding = 0) uniform sampler2D tex;

layout(push_constant) uniform Push {
    mat4 viewProj;
    vec4 tint;   // rgba-модуляция цвета
    vec4 params; // x = alphaRef, y = ambient, z = diffuse, w = unused
} pc;

void main() {
    vec4 sampleColor = texture(tex, fract(inUV));
    if (sampleColor.a < pc.params.x) {
        discard;
    }
    vec3 n = normalize(inNormal);
    vec3 lightDir = normalize(vec3(0.35, -0.50, 0.78));
    float lambert = max(dot(n, lightDir), 0.0);
    float shade = min(pc.params.y + pc.params.z * lambert, 1.0);
    vec3 col = sampleColor.rgb * pc.tint.rgb * shade;
    outColor = vec4(col, pc.tint.a);
}

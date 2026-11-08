#version 450

// RGBA-атлас хранит иконки/текстуры интерфейса и белые glyph masks.
layout(location = 0) in vec2 inUV;
layout(location = 1) in vec4 inColor;

layout(location = 0) out vec4 outColor;

layout(set = 0, binding = 0) uniform sampler2D tex;

void main() {
    vec4 texel = texture(tex, inUV);
    outColor = vec4(inColor.rgb * texel.rgb, inColor.a * texel.a);
}

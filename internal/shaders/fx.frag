#version 450

// Частица: мягкая текстура-спрайт, умноженная на цвет вершины (в нём уже
// затухание по возрасту). Смешивание задаёт пайплайн: альфа для дыма и
// пыли, сложение для вспышек и искр.
layout(location = 0) in vec2 inUV;
layout(location = 1) in vec4 inColor;

layout(location = 0) out vec4 outColor;

layout(set = 0, binding = 0) uniform sampler2D tex;

void main() {
    vec4 sprite = texture(tex, inUV);
    outColor = vec4(sprite.rgb * inColor.rgb, sprite.a * inColor.a);
}

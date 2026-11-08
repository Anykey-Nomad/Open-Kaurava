-- Смоук-тест Lua-хуков: on_update и on_input в кадровом цикле движка.
local updates = 0
local inputs = 0

on_update(function(dt)
	updates = updates + 1
	if updates == 5 then
		print(string.format("[lua] on_update: 5 кадров, последний dt=%.4f", dt))
	end
end)

on_input(function(ev)
	inputs = inputs + 1
	print(string.format("[lua] on_input: kind=%s key=%s x=%d y=%d", ev.kind, ev.key, ev.x, ev.y))
end)

print("[lua] скрипт загружен: подписки оформлены")

# Скриншот окна движка: находит окно по классу/заголовку, выполняет ввод
# (мышь, колесо, клавиши) и сохраняет снимок окна.
#
#   powershell -File tools/scene_shot.ps1 -Title "purego Vulkan" `
#       -Out "screenshots/menu.png" -MoveX 640 -MoveY 400 -Key ESC
#
# Важно: процесс делается DPI-осведомлённым, иначе GetWindowRect/SetCursorPos
# работают в виртуализированных координатах и попадают мимо окна.
param(
  [string]$Title = "Open-Kaurava",
  [string]$Class = "Open-KauravaWindowClass",
  [string]$Out = "screenshots/scene.png",
  [int]$MoveX = -1,   # -1 — мышь не двигать
  [int]$MoveY = -1,
  [int]$ClickX = -1,  # -1 — не кликать
  [int]$ClickY = -1,
  [int]$SettleMs = 900,
  [int]$DragDX = 0,   # перетаскивание мышью на (DragDX, DragDY) px
  [int]$DragDY = 0,
  [int]$Scroll = 0,   # щелчков колеса (>0 — вверх)
  [string]$Key = ""   # клавиша: ESC, ENTER, SPACE, UP, DOWN, LEFT, RIGHT, A-Z, 0-9
)

$ErrorActionPreference = "Stop"
Add-Type -AssemblyName System.Drawing

Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Text;
public class M {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, UIntPtr dwExtraInfo);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool GetClientRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern bool ClientToScreen(IntPtr h, ref POINT p);
  [DllImport("user32.dll")] public static extern bool PrintWindow(IntPtr h, IntPtr dc, uint flags);
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr h);
  [DllImport("user32.dll")] public static extern bool BringWindowToTop(IntPtr h);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr h, int cmd);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr FindWindowW(string cls, string title);
  [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr h, uint msg, IntPtr wp, IntPtr lp);
  [DllImport("user32.dll")] public static extern void keybd_event(byte vk, byte scan, uint flags, UIntPtr extra);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr lParam);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowTextW(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetClassNameW(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  public delegate bool EnumProc(IntPtr h, IntPtr lParam);
  public struct RECT { public int Left, Top, Right, Bottom; }
  public struct POINT { public int X, Y; }

  // Ищет видимое окно по подстроке заголовка и (если задан) по классу.
  public static IntPtr FindBySubstring(string needle, string appClass) {
    IntPtr found = IntPtr.Zero;
    EnumWindows((h, l) => {
      if (!IsWindowVisible(h)) return true;
      var sb = new StringBuilder(512);
      GetWindowTextW(h, sb, 512);
      if (!sb.ToString().Contains(needle)) return true;
      if (appClass != null && appClass.Length > 0) {
        var cls = new StringBuilder(256);
        GetClassNameW(h, cls, 256);
        if (cls.ToString() != appClass) return true;
      }
      found = h; return false;
    }, IntPtr.Zero);
    return found;
  }

  // Клиентские координаты → экранные.
  public static int ScreenX(IntPtr h, int x) {
    POINT p = new POINT(); p.X = x; p.Y = 0;
    ClientToScreen(h, ref p);
    return p.X;
  }
  public static int ScreenY(IntPtr h, int y) {
    POINT p = new POINT(); p.X = 0; p.Y = y;
    ClientToScreen(h, ref p);
    return p.Y;
  }

  // Поднимает окно на передний план: ALT «разблокирует» SetForegroundWindow.
  public static void Focus(IntPtr h) {
    keybd_event(0x12, 0, 0, UIntPtr.Zero);
    keybd_event(0x12, 0, 2, UIntPtr.Zero);
    BringWindowToTop(h);
    SetForegroundWindow(h);
  }
}
"@

# DPI-осведомлённость: координаты окон и курсора — в физических пикселях.
[void][M]::SetProcessDPIAware()

$hwnd = [IntPtr]::Zero
for ($i = 0; $i -lt 40; $i++) {
  $hwnd = [M]::FindBySubstring($Title, $Class)
  if ($hwnd -ne [IntPtr]::Zero) { break }
  Start-Sleep -Milliseconds 500
}
if ($hwnd -eq [IntPtr]::Zero) { Write-Output "NO_WINDOW"; exit 1 }

[void][M]::ShowWindow($hwnd, 9)      # SW_RESTORE
[void][M]::Focus($hwnd)
Start-Sleep -Milliseconds 300

$rect = New-Object M+RECT
[void][M]::GetWindowRect($hwnd, [ref]$rect)
$w = $rect.Right - $rect.Left
$h = $rect.Bottom - $rect.Top
Write-Output ("window: {0},{1} - {2},{3} ({4}x{5})" -f $rect.Left, $rect.Top, $rect.Right, $rect.Bottom, $w, $h)

# Начало клиентской области в координатах снимка — чтобы проверять
# попадание по пикселям.
$ox = [M]::ScreenX($hwnd, 0) - $rect.Left
$oy = [M]::ScreenY($hwnd, 0) - $rect.Top
Write-Output ("client origin in shot: {0},{1}" -f $ox, $oy)

if ($MoveX -ge 0 -and $MoveY -ge 0) {
  $sx = [M]::ScreenX($hwnd, $MoveX)
  $sy = [M]::ScreenY($hwnd, $MoveY)
  [void][M]::SetCursorPos($sx, $sy)
  Start-Sleep -Milliseconds 250
  Write-Output ("mouse -> screen {0},{1} (client {2},{3})" -f $sx, $sy, $MoveX, $MoveY)
}

if ($ClickX -ge 0 -and $ClickY -ge 0) {
  $sx = [M]::ScreenX($hwnd, $ClickX)
  $sy = [M]::ScreenY($hwnd, $ClickY)
  [void][M]::SetCursorPos($sx, $sy)
  Start-Sleep -Milliseconds 150
  [M]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)   # LEFTDOWN
  Start-Sleep -Milliseconds 60
  [M]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)   # LEFTUP
  Write-Output "click at client $ClickX,$ClickY"
}

# Перетаскивание: зажать левую кнопку и сдвинуть мышь.
if ($DragDX -ne 0 -or $DragDY -ne 0) {
  $sx = [M]::ScreenX($hwnd, $MoveX)
  $sy = [M]::ScreenY($hwnd, $MoveY)
  [void][M]::SetCursorPos($sx, $sy)
  Start-Sleep -Milliseconds 120
  [M]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)   # LEFTDOWN
  $steps = 12
  for ($i = 1; $i -le $steps; $i++) {
    [void][M]::SetCursorPos($sx + [int]($DragDX * $i / $steps), $sy + [int]($DragDY * $i / $steps))
    Start-Sleep -Milliseconds 30
  }
  [M]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)   # LEFTUP
  Write-Output "drag $DragDX,$DragDY from client $MoveX,$MoveY"
}

# Колесо мыши: WHEEL_DELTA = 120 за щелчок. Событие отправляем прямо в окно
# (mouse_event с колесом доставляется не туда, если окно перекрыто).
if ($Scroll -ne 0) {
  $sx = [M]::ScreenX($hwnd, $MoveX)
  $sy = [M]::ScreenY($hwnd, $MoveY)
  $lp = [IntPtr](($sy -shl 16) -bor ($sx -band 0xFFFF))
  for ($i = 0; $i -lt [Math]::Abs($Scroll); $i++) {
    $delta = if ($Scroll -gt 0) { 120 } else { -120 }
    $wp = [IntPtr]($delta -shl 16)
    [void][M]::PostMessage($hwnd, 0x020A, $wp, $lp)   # WM_MOUSEWHEEL
    Start-Sleep -Milliseconds 200
  }
  Write-Output "scroll $Scroll"
}

# Клавиша: WM_KEYDOWN/WM_KEYUP прямо в окно — работает без фокуса.
if ($Key -ne "") {
  $codes = @{ ESC = 0x1B; ENTER = 0x0D; SPACE = 0x20; UP = 0x26; DOWN = 0x28; LEFT = 0x25; RIGHT = 0x27 }
  $vk = 0
  if ($codes.ContainsKey($Key.ToUpper())) {
    $vk = $codes[$Key.ToUpper()]
  } elseif ($Key.Length -eq 1) {
    $vk = [byte][char]::ToUpperInvariant($Key[0])
  }
  if ($vk -ne 0) {
    [void][M]::PostMessage($hwnd, 0x0100, [IntPtr]$vk, [IntPtr]0)   # WM_KEYDOWN
    Start-Sleep -Milliseconds 80
    [void][M]::PostMessage($hwnd, 0x0101, [IntPtr]$vk, [IntPtr]0)   # WM_KEYUP
    Write-Output "key $Key"
  } else {
    Write-Output "unknown key $Key"
  }
  Start-Sleep -Milliseconds 400
}

Start-Sleep -Milliseconds $SettleMs

# Снимок окна: PrintWindow рисует окно даже когда оно перекрыто.
$b = [System.Drawing.Bitmap]::new($w, $h)
$g = [System.Drawing.Graphics]::FromImage($b)
$ok = $false
for ($try = 0; $try -lt 3 -and -not $ok; $try++) {
  $hdc = $g.GetHdc()
  $ok = [M]::PrintWindow($hwnd, $hdc, 2)   # PW_RENDERFULLCONTENT
  $g.ReleaseHdc($hdc)
  if (-not $ok) { Start-Sleep -Milliseconds 400 }
}
if (-not $ok) {
  Write-Output "PrintWindow failed, fallback to screen"
  [void][M]::Focus($hwnd)
  Start-Sleep -Milliseconds 300
  $g.CopyFromScreen($rect.Left, $rect.Top, 0, 0, $b.Size)
}
$b.Save($Out)
$g.Dispose(); $b.Dispose()
Write-Output "screenshot: $Out"

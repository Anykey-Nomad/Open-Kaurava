# Mouse-input test for the demo: click, drag with selection frame, right click
# + screenshot mid-drag. Finds the window by exact title, so it works for both
# the native Windows build and a WSLg-hosted Linux build.
$ErrorActionPreference = "Stop"

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
Add-Type @"
using System;
using System.Runtime.InteropServices;
public class M {
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int X, int Y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint dwFlags, int dx, int dy, uint dwData, UIntPtr dwExtraInfo);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern IntPtr FindWindowW(string cls, string title);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr lParam);
  [DllImport("user32.dll", CharSet = CharSet.Unicode)] public static extern int GetWindowTextW(IntPtr h, System.Text.StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  public struct RECT { public int Left, Top, Right, Bottom; }

  // WSLg prefixes the X11 title, so we match by substring.
  public static IntPtr FindBySubstring(string needle) {
    IntPtr found = IntPtr.Zero;
    EnumWindows((h, l) => {
      if (!IsWindowVisible(h)) return true;
      var sb = new System.Text.StringBuilder(512);
      GetWindowTextW(h, sb, 512);
      if (sb.ToString().Contains(needle)) { found = h; return false; }
      return true;
    }, IntPtr.Zero);
    return found;
  }
}
"@

# Window may take a while to appear (WSLg boot), poll for up to 20 s.
$hwnd = [IntPtr]::Zero
for ($i = 0; $i -lt 40; $i++) {
    $hwnd = [M]::FindBySubstring("Open-Kaurava input-test")
    if ($hwnd -ne [IntPtr]::Zero) { break }
    Start-Sleep -Milliseconds 500
}
if ($hwnd -eq [IntPtr]::Zero) { Write-Output "NO_WINDOW"; exit 1 }

$rect = New-Object M+RECT
[void][M]::GetWindowRect($hwnd, [ref]$rect)
Write-Output ("window: {0},{1} - {2},{3}" -f $rect.Left, $rect.Top, $rect.Right, $rect.Bottom)
$cx = [int](($rect.Left + $rect.Right) / 2)
$cy = [int](($rect.Top + $rect.Bottom) / 2)

# 1. Left click at the center
[M]::SetCursorPos($cx, $cy)
Start-Sleep -Milliseconds 150
[M]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)   # LEFTDOWN
Start-Sleep -Milliseconds 60
[M]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)   # LEFTUP
Start-Sleep -Milliseconds 300

# 2. Drag: press top-left, pull down-right
$x0 = $cx - 150; $y0 = $cy - 100
[M]::SetCursorPos($x0, $y0)
Start-Sleep -Milliseconds 150
[M]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)   # LEFTDOWN
for ($i = 1; $i -le 10; $i++) {
    [M]::SetCursorPos($x0 + $i * 30, $y0 + $i * 20)
    Start-Sleep -Milliseconds 40
}
Start-Sleep -Milliseconds 150

# Screenshot while the drag frame is active
$w = $rect.Right - $rect.Left
$h = $rect.Bottom - $rect.Top
$b = [System.Drawing.Bitmap]::new($w, $h)
$g = [System.Drawing.Graphics]::FromImage($b)
$g.CopyFromScreen($rect.Left, $rect.Top, 0, 0, $b.Size)
$b.Save("C:/Users/anyke/go/src/github.com/Anykey-Nomad/Open-Kaurava/screenshots/drag-live.png")
$g.Dispose(); $b.Dispose()
Write-Output "screenshot drag taken"

[M]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)   # LEFTUP - finish drag
Start-Sleep -Milliseconds 300

# 3. Right click
[M]::mouse_event(0x0008, 0, 0, 0, [UIntPtr]::Zero)   # RIGHTDOWN
Start-Sleep -Milliseconds 60
[M]::mouse_event(0x0010, 0, 0, 0, [UIntPtr]::Zero)   # RIGHTUP
Start-Sleep -Milliseconds 300

Write-Output "input sent"

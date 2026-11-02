# 生成 LLM Gateway Proxy 图标集：
#   build/appicon.png      512x512 应用图标
#   build/windows/icon.ico 多尺寸 exe 图标（16/24/32/48/64/128/256）
#   build/tray.ico         32x32 托盘图标
# 设计：深色圆角底（#21262d→#0d1117）+ 蓝色闪电（#58a6ff→#1f6feb），呼应应用 UI 主题
Add-Type -AssemblyName System.Drawing

$ErrorActionPreference = 'Stop'
$buildDir = 'E:\dev\test-workspace\claude-code\llm-gateway-proxy\build'

function New-AppBitmap([int]$size) {
    $bmp = New-Object System.Drawing.Bitmap($size, $size)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $s = $size / 512.0  # 坐标缩放因子（按 512 画布设计）

    # ── 圆角矩形路径（半径 110）──
    $r = [math]::Round(110 * $s)
    $path = New-Object System.Drawing.Drawing2D.GraphicsPath
    $d = 2 * $r
    $path.AddArc($r, 0, $d, $d, 180, 90)                # 左上
    $path.AddArc($size - $d - $r, 0, $d, $d, 270, 90)   # 右上
    $path.AddArc($size - $d - $r, $size - $d - $r, $d, $d, 0, 90)   # 右下
    $path.AddArc($r, $size - $d - $r, $d, $d, 90, 90)   # 左下
    $path.CloseFigure()

    # ── 背景：对角渐变 ──
    $bgRect = New-Object System.Drawing.Rectangle(0, 0, $size, $size)
    $bg = New-Object System.Drawing.Drawing2D.LinearGradientBrush(
        $bgRect,
        [System.Drawing.Color]::FromArgb(255, 33, 38, 45),    # #21262d
        [System.Drawing.Color]::FromArgb(255, 13, 17, 23),    # #0d1117
        [System.Drawing.Drawing2D.LinearGradientMode]::BackwardDiagonal)
    $g.FillPath($bg, $path)

    # ── 边框 ──
    $pen = New-Object System.Drawing.Pen([System.Drawing.Color]::FromArgb(255, 48, 54, 61), [math]::Max(1, [math]::Round(3 * $s)))
    $g.DrawPath($pen, $path)

    # ── 闪电多边形（512 画布坐标）──
    $bolt = @(
        @{x=302; y=72}, @{x=168; y=292}, @{x=248; y=292},
        @{x=206; y=444}, @{x=372; y=228}, @{x=284; y=228}, @{x=348; y=72}
    )
    $pts = New-Object System.Drawing.PointF[] ($bolt.Count)
    for ($i = 0; $i -lt $bolt.Count; $i++) {
        $pts[$i] = New-Object System.Drawing.PointF(($bolt[$i].x * $s), ($bolt[$i].y * $s))
    }
    $boltRect = New-Object System.Drawing.RectangleF((168 * $s), (72 * $s), (204 * $s), (372 * $s))
    $boltBrush = New-Object System.Drawing.Drawing2D.LinearGradientBrush(
        $boltRect,
        [System.Drawing.Color]::FromArgb(255, 88, 166, 255),  # #58a6ff
        [System.Drawing.Color]::FromArgb(255, 31, 111, 235),  # #1f6feb
        [System.Drawing.Drawing2D.LinearGradientMode]::Vertical)
    $g.FillPolygon($boltBrush, $pts)

    # ── 闪电高光（左半透明叠加，增强立体感）──
    $shine = @(
        @{x=302; y=72}, @{x=168; y=292}, @{x=248; y=292}, @{x=232; y=350}
    )
    $spts = New-Object System.Drawing.PointF[] ($shine.Count)
    for ($i = 0; $i -lt $shine.Count; $i++) {
        $spts[$i] = New-Object System.Drawing.PointF(($shine[$i].x * $s), ($shine[$i].y * $s))
    }
    $alpha = [int](70)
    $shineBrush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::FromArgb($alpha, 230, 240, 255))
    $g.FillPolygon($shineBrush, $spts)

    $g.Dispose()
    return $bmp
}

function Save-PngBytes($bmp) {
    $ms = New-Object System.IO.MemoryStream
    $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    return $ms.ToArray()
}

# BMP（32bit BGRA，底顶行序）+ AND mask —— ICO 小尺寸 entry 用
function ConvertTo-BmpEntry($bmp) {
    $w = $bmp.Width; $h = $bmp.Height
    $rect = New-Object System.Drawing.Rectangle(0, 0, $w, $h)
    $data = $bmp.LockBits($rect, [System.Drawing.Imaging.ImageLockMode]::ReadOnly, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $stride = [math]::Abs($data.Stride)
    $raw = New-Object byte[] ($stride * $h)
    [System.Runtime.InteropServices.Marshal]::Copy($data.Scan0, $raw, 0, $raw.Length)
    $bmp.UnlockBits($data)

    # 行序翻转为底→顶，BGRA 已是 ICO 需要的字节序（little-endian ARGB = BGRA in memory）
    $xor = New-Object byte[] ($w * 4 * $h)
    for ($y = 0; $y -lt $h; $y++) {
        $srcOff = $y * $stride
        $dstOff = ($h - 1 - $y) * $w * 4
        [Array]::Copy($raw, $srcOff, $xor, $dstOff, $w * 4)
    }
    $andRow = [math]::Floor(($w + 31) / 32) * 4
    $and = New-Object byte[] ($andRow * $h)  # 全 0（32bit alpha 已含透明度）

    $header = New-Object System.IO.MemoryStream
    $bw = New-Object System.IO.BinaryWriter($header)
    $bw.Write([uint32]40)             # biSize
    $bw.Write([int32]$w)              # biWidth
    $bw.Write([int32]($h * 2))        # biHeight（XOR+AND）
    $bw.Write([uint16]1)              # biPlanes
    $bw.Write([uint16]32)             # biBitCount
    $bw.Write([uint32]0)              # biCompression
    $bw.Write([uint32]($xor.Length + $and.Length))
    $bw.Write([int32]0); $bw.Write([int32]0)  # ppm
    $bw.Write([uint32]0); $bw.Write([uint32]0)  # colors used / important
    $bw.Write($xor)
    $bw.Write($and)
    $bw.Flush()
    return $header.ToArray()
}

# 多尺寸 PNG-in-ICO（256/128）+ BMP-in-ICO（小尺寸）
function New-IcoFile($sizes, $outPath) {
    $entries = @()
    foreach ($sz in $sizes) {
        $bmp = New-AppBitmap $sz
        if ($sz -ge 128) { $payload = Save-PngBytes $bmp; $isPng = $true }
        else { $payload = ConvertTo-BmpEntry $bmp; $isPng = $false }
        $entries += ,@($sz, $payload, $isPng)
        $bmp.Dispose()
    }

    $fs = [System.IO.File]::Create($outPath)
    $w = New-Object System.IO.BinaryWriter($fs)
    $w.Write([uint16]0); $w.Write([uint16]1); $w.Write([uint16]$entries.Count)
    $offset = 6 + 16 * $entries.Count
    foreach ($e in $entries) {
        $sz = $e[0]; $payload = $e[1]
        $w.Write([byte]$(if ($sz -ge 256) { 0 } else { $sz }))  # 宽
        $w.Write([byte]$(if ($sz -ge 256) { 0 } else { $sz }))  # 高
        $w.Write([byte]0); $w.Write([byte]0)
        $w.Write([uint16]1); $w.Write([uint16]32)
        $w.Write([uint32]$payload.Length)
        $w.Write([uint32]$offset)
        $offset += $payload.Length
    }
    foreach ($e in $entries) { $w.Write([byte[]]$e[1]) }
    $w.Close()
}

# ── 生成 ──
$png512 = New-AppBitmap 512
$png512.Save("$buildDir\appicon.png", [System.Drawing.Imaging.ImageFormat]::Png)
$png512.Dispose()

New-IcoFile @(256, 128, 64, 48, 32, 24, 16) "$buildDir\windows\icon.ico"
New-IcoFile @(32, 16) "$buildDir\tray.ico"

Write-Host "OK: appicon.png / windows/icon.ico / tray.ico 已生成"

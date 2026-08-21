Add-Type -AssemblyName System.Drawing

$sizes = 16, 32, 48, 128
$iconDirectory = $PSScriptRoot

foreach ($size in $sizes) {
    $bitmap = [System.Drawing.Bitmap]::new($size, $size, [System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
    $graphics.Clear([System.Drawing.Color]::Transparent)
    $graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::AntiAlias
    $graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $graphics.ScaleTransform($size / 128.0, $size / 128.0)

    $tray = [System.Drawing.Drawing2D.GraphicsPath]::new()
    $tray.AddPolygon([System.Drawing.PointF[]]@(
        [System.Drawing.PointF]::new(16, 56),
        [System.Drawing.PointF]::new(30, 70),
        [System.Drawing.PointF]::new(30, 88),
        [System.Drawing.PointF]::new(31, 91),
        [System.Drawing.PointF]::new(34, 94),
        [System.Drawing.PointF]::new(37, 95),
        [System.Drawing.PointF]::new(91, 95),
        [System.Drawing.PointF]::new(94, 94),
        [System.Drawing.PointF]::new(97, 91),
        [System.Drawing.PointF]::new(98, 88),
        [System.Drawing.PointF]::new(98, 70),
        [System.Drawing.PointF]::new(112, 56),
        [System.Drawing.PointF]::new(112, 111),
        [System.Drawing.PointF]::new(16, 111)
    ))
    $trayBrush = [System.Drawing.SolidBrush]::new([System.Drawing.ColorTranslator]::FromHtml('#24313a'))
    $graphics.FillPath($trayBrush, $tray)

    $arrow = [System.Drawing.Drawing2D.GraphicsPath]::new()
    $arrow.AddArc(54, 8, 20, 20, 180, 180)
    $arrow.AddLine(74, 18, 74, 58)
    $arrow.AddLine(92, 58, 64, 87)
    $arrow.AddLine(36, 58, 54, 58)
    $arrow.CloseFigure()
    $arrowBrush = [System.Drawing.SolidBrush]::new([System.Drawing.ColorTranslator]::FromHtml('#397a73'))
    $graphics.FillPath($arrowBrush, $arrow)

    $target = Join-Path $iconDirectory "icon-$size.png"
    $bitmap.Save($target, [System.Drawing.Imaging.ImageFormat]::Png)

    $arrowBrush.Dispose()
    $arrow.Dispose()
    $trayBrush.Dispose()
    $tray.Dispose()
    $graphics.Dispose()
    $bitmap.Dispose()
}

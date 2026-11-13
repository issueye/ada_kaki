# Build AdaKaki as a Windows GUI executable: no console window appears
# when it starts. The icon comes from rsrc_windows_*.syso, linked by go
# build automatically.
$ErrorActionPreference = 'Stop'
go build -trimpath -ldflags '-H windowsgui' -o adakaki.exe .
Write-Host 'built adakaki.exe'

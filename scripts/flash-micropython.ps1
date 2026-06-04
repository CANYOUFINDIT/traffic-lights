param(
    [Parameter(Mandatory = $true)]
    [string]$Port
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

python -m esptool --chip esp32c3 --port $Port erase-flash
python -m esptool --chip esp32c3 --port $Port --baud 460800 write-flash -z 0x0 firmware/ESP32_GENERIC_C3-20260406-v1.28.0.bin

& "$Root\scripts\upload-firmware.ps1" -Port $Port

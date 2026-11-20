param([string]$Executable, [ValidateSet('before','used')][string]$Mode,
      [ValidateSet('candidate','guard')][string]$Variant)
$ErrorActionPreference = 'Stop'
$names = @('Tailscale-In','Tailscale-Process')
$sentinel = "$env:SystemRoot\System32\notepad.exe"
function Snapshot {
    $rows = @(Get-NetFirewallRule -DisplayName $names -ErrorAction SilentlyContinue |
        Sort-Object DisplayName,Name | ForEach-Object {
            $app = $_ | Get-NetFirewallApplicationFilter
            $port = $_ | Get-NetFirewallPortFilter
            [pscustomobject]@{ name=$_.Name; display=$_.DisplayName; action=$_.Action.ToString(); enabled=$_.Enabled.ToString(); direction=$_.Direction.ToString(); profile=$_.Profile.ToString(); program=$app.Program; protocol=$port.Protocol; localport=[string]$port.LocalPort }
        })
    ConvertTo-Json -InputObject $rows -Depth 4 -Compress
}
if ((Snapshot) -ne '[]' -or (Get-Service -Name Tailscale -ErrorAction SilentlyContinue)) {
    throw 'Disposable host already has a Tailscale service or firewall rule'
}
try {
    foreach ($n in $names) {
        New-NetFirewallRule -Name "Headwire-Review-$n" -DisplayName $n -Direction Inbound -Action Block -Enabled False -Program $sentinel -Profile Private -Protocol UDP -LocalPort 47414 | Out-Null
    }
    $before = Snapshot
    if (@(ConvertFrom-Json $before).Count -ne 2) { throw 'sentinel rule setup failed' }
    $env:HEADWIRE_FIREWALL_PROBE = $Mode
    & $Executable '-test.v' '-test.run=^TestNativeFirewallOwnership$' '-test.timeout=3m'
    if ($LASTEXITCODE -ne 0) { throw "Native probe failed: $LASTEXITCODE" }
    $after = Snapshot
    $preserved = $before -ceq $after
    Write-Host "RESULT $Variant $Mode preserved=$preserved"
    Write-Host "BEFORE $before"
    Write-Host "AFTER $after"
    if ($Variant -eq 'guard' -and $Mode -eq 'before') {
        if (!$preserved) { throw 'unused router changed seeded host rules' }
    } else {
        $rows = @(ConvertFrom-Json $after)
        if ($preserved -or $rows.Count -ne 1 -or $rows[0].display -ne 'Tailscale-Process' -or $rows[0].program -ine (Resolve-Path $Executable).Path) {
            throw 'expected native netsh to replace the sentinel rules with the current process rule'
        }
    }
} finally {
    Get-NetFirewallRule -DisplayName $names -ErrorAction SilentlyContinue | Remove-NetFirewallRule
}

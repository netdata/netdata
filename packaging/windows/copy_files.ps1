
Function NetdataCopyConfig {
    param ($dst, $src, $file)

    Write-Host "Creating $file if it does not exist!"

    $testDST = "$dst\$file"
    $testSRC = "$src\$file"
    if (-Not (Test-Path $testDST)) {
        if (Test-Path $testSRC) {
            robocopy /xc /xn /xo $src $dst $file
        }
    }
}

Function NetdataDownloadNetdataConfig {
    param ($path)

    Write-Host "Creating netdata.conf if it does not exist!"

    $netdataConfPATH = "$path\netdata.conf"
    $netdataConfURL = "http://localhost:19999/netdata.conf"
    if (Test-Path $netdataConfPATH) {
        exit 0
    }

    try {
        Invoke-WebRequest $netdataConfURL -OutFile $netdataConfPATH
    }
    catch {
        New-Item -Path "$netdataConfPATH" -ItemType File
    }
}

$confPath = "C:\Program Files\Netdata\etc\netdata";
$stockStreamPath = "C:\Program Files\Netdata\usr\lib\netdata\conf.d";

NetdataCopyConfig $confPath $stockStreamPath "stream.conf"

NetdataDownloadNetdataConfig $confPath

exit 0;


Function NetdataCopyConfig {
    param ($dst, $src, $file)
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

    $netdataConfPATH = "$path\netdata.conf"
    $netdataConfURL = "http://localhost:19999/netdata.conf"
    if (Test-Path $netdataConfPATH) {
        exit 0
    }

    Invoke-WebRequest $netdataConfURL -OutFile $netdataConfPATH
    if (Test-Path $netdataConfPATH) {
        exit 0
    }

    New-Item -Path "$netdataConfPATH" -ItemType File
}

$confPath = "C:\Program Files\Netdata\etc\netdata";
$stockStreamPath = "C:\Program Files\Netdata\usr\lib\netdata\conf.d";

NetdataCopyConfig $confPath $stockStreamPath "stream.conf"

NetdataDownloadNetdataConfig $confPath

exit 0;


Function NetdataCopyConfig {
    param ($dst, $src, $file)
    $testDST = "$dst\$file"
    $testSRC = "$src\$file"
    if (-Not (Test-Path $testDST)) {
        if (Test-Path $testSRC) {
            robocopy /xc /xn /xo $src $dst $file
            exit 0;
        } else {
            exit 1;
        }
    }
}

$streamPath = "C:\Program Files\Netdata\etc\netdata";
$stockStreamPath = "C:\Program Files\Netdata\usr\lib\netdata\conf.d";

NetdataCopyConfig $streamPath $stockStreamPath "stream.conf"

exit 0;

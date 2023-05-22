$ErrorActionPreference = "Stop";
trap { $host.SetShouldExit(1) }

Debug "$(gci env:* | sort-object name | Out-String)"
Configure-Groot "$env:WINC_TEST_ROOTFS"
Configure-Winc-Network delete

ginkgo.exe -p -r --race --keep-going --randomize-suites --fail-on-pending
if ($LastExitCode -ne 0) {
  throw "tests failed"
}

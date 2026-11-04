Unicode true
!ifndef OUTPUT
!error "Provide -DOUTPUT=..."
!endif
!ifndef FIXTURE_PATH
!error "Provide -DFIXTURE_PATH=..."
!endif
Name "Payload audit test only"
OutFile "${OUTPUT}"
Section
  SetOutPath "$INSTDIR"
  File "${FIXTURE_PATH}\Planet Windows.exe"
  File "${FIXTURE_PATH}\ipfs.exe"
  File "${FIXTURE_PATH}\icon.png"
  File "${FIXTURE_PATH}\kubo-LICENSE.txt"
  File "${FIXTURE_PATH}\kubo-LICENSE-MIT.txt"
  File "${FIXTURE_PATH}\kubo-LICENSE-APACHE.txt"
  File "${FIXTURE_PATH}\LICENSE-PLANET.txt"
  File "${FIXTURE_PATH}\THIRD-PARTY-NOTICES.txt"
  File "${FIXTURE_PATH}\KUBO-BUILDINFO.txt"
  File "${FIXTURE_PATH}\KUBO-DEPENDENCY-LICENSES.txt"
SectionEnd

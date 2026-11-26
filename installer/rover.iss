; Inno Setup script for Rover (formerly Dilreet).
;   Standard:  iscc /DAppVersion=0.8.0 installer\rover.iss
;   Offline:   go run ./tools/mkpack -model qwen3-1.7b -runtimes cpu,vulkan -stage installer\offline
;              iscc /DAppVersion=0.8.0 /DOffline=Light installer\rover.iss
;   The offline build bundles the Jiv brain (model + llama.cpp runtimes) and is
;   split into slices under 2 GB (Rover-Offline-Setup-<ver>.exe + -N.bin) when
;   it is bigger than that, so every file fits a GitHub release asset.
#ifndef AppVersion
  #define AppVersion "0.8.0"
#endif
[Setup]
; Same AppId as Dilreet: Rover installs over Dilreet 0.6/0.7 as an upgrade.
AppId={{8C4C3B9E-5D7A-4A8E-9F3B-D1E2E7A0B3C1}
AppName=Rover
AppVersion={#AppVersion}
AppVerName=Rover {#AppVersion}
AppPublisher=Rover
DefaultDirName={localappdata}\Programs\Rover
UsePreviousAppDir=yes
DefaultGroupName=Rover
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir=..\dist
#ifdef Offline
OutputBaseFilename=Rover-Offline-Setup-{#AppVersion}
DiskSpanning=yes
DiskSliceSize=2000000000
SlicesPerDisk=1
Compression=lzma2/fast
#else
OutputBaseFilename=Rover-Setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
#endif
SetupIconFile=..\winres\icon.ico
UninstallDisplayIcon={app}\rover.exe
UninstallDisplayName=Rover
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
CloseApplications=yes

[InstallDelete]
; Left over from Dilreet (the program, its shortcuts).
Type: files; Name: "{app}\dilreet.exe"
Type: files; Name: "{app}\dilreet.exe.old"
Type: files; Name: "{autoprograms}\Dilreet.lnk"
Type: files; Name: "{autodesktop}\Dilreet.lnk"

[Files]
Source: "..\rover.exe"; DestDir: "{app}"; Flags: ignoreversion
#ifdef Offline
; Checked against the SHA-256 values built into Rover before it uses them.
Source: "offline\*"; DestDir: "{localappdata}\Rover\Brain"; Flags: ignoreversion recursesubdirs nocompression
Source: "licenses\*"; DestDir: "{localappdata}\Rover\Brain\licenses"; Flags: ignoreversion
#endif

[Icons]
Name: "{autoprograms}\Rover"; Filename: "{app}\rover.exe"
Name: "{autodesktop}\Rover"; Filename: "{app}\rover.exe"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; Flags: unchecked
Name: "explorermenu"; Description: "Add ""Deploy with Rover"" to the right-click menu of folders and .zip files"

[Registry]
; The Dilreet right-click entries are replaced by the Rover ones.
Root: HKCU; Subkey: "Software\Classes\Directory\shell\DilreetDeploy"; ValueType: none; Flags: deletekey
Root: HKCU; Subkey: "Software\Classes\Directory\Background\shell\DilreetDeploy"; ValueType: none; Flags: deletekey
Root: HKCU; Subkey: "Software\Classes\SystemFileAssociations\.zip\shell\DilreetDeploy"; ValueType: none; Flags: deletekey
; Right-click a folder, the background of an open folder, or a .zip file -> Deploy with Rover.
; Per-user (HKCU), removed on uninstall. Rover then asks which project to deploy to.
Root: HKCU; Subkey: "Software\Classes\Directory\shell\RoverDeploy"; ValueType: string; ValueName: ""; ValueData: "Deploy with Rover"; Flags: uninsdeletekey; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\Directory\shell\RoverDeploy"; ValueType: string; ValueName: "Icon"; ValueData: "{app}\rover.exe"; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\Directory\shell\RoverDeploy\command"; ValueType: string; ValueName: ""; ValueData: """{app}\rover.exe"" -deploy ""%1"""; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\Directory\Background\shell\RoverDeploy"; ValueType: string; ValueName: ""; ValueData: "Deploy this folder with Rover"; Flags: uninsdeletekey; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\Directory\Background\shell\RoverDeploy"; ValueType: string; ValueName: "Icon"; ValueData: "{app}\rover.exe"; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\Directory\Background\shell\RoverDeploy\command"; ValueType: string; ValueName: ""; ValueData: """{app}\rover.exe"" -deploy ""%V"""; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\SystemFileAssociations\.zip\shell\RoverDeploy"; ValueType: string; ValueName: ""; ValueData: "Deploy with Rover"; Flags: uninsdeletekey; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\SystemFileAssociations\.zip\shell\RoverDeploy"; ValueType: string; ValueName: "Icon"; ValueData: "{app}\rover.exe"; Tasks: explorermenu
Root: HKCU; Subkey: "Software\Classes\SystemFileAssociations\.zip\shell\RoverDeploy\command"; ValueType: string; ValueName: ""; ValueData: """{app}\rover.exe"" -deploy ""%1"""; Tasks: explorermenu
; The Dilreet "start with Windows" entry; Rover writes its own when that setting is on.
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: none; ValueName: "Dilreet"; Flags: deletevalue

[Run]
Filename: "{app}\rover.exe"; Description: "Start Rover"; Flags: nowait postinstall skipifsilent
; Self-update: Rover runs this installer with /VERYSILENT and closes itself; start it again afterwards.
Filename: "{app}\rover.exe"; Parameters: "-after-update"; Flags: nowait skipifnotsilent

; Your projects (Documents\Rover) and settings (%APPDATA%\Rover) are kept on uninstall.
; Rover moves Documents\Dilreet and %APPDATA%\Dilreet there by itself on first start.

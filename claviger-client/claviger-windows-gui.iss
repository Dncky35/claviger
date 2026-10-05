[Setup]
; Basic App Info
AppName=Claviger Client
AppVersion=Claviger - v0.4.3
AppPublisher="Cloudrocean"
AppPublisherURL="https://claviger.cloudrocean.com"

; Set the default installation folder to C:\Program Files\Claviger
DefaultDirName={autopf}\Claviger
DefaultGroupName=Claviger

; Disable the clunky "Select Start Menu Folder" page to just use a modern checkbox
DisableProgramGroupPage=yes

; The name of the final installer file
OutputBaseFilename=ClavigerClient_GUI_amd64
OutputDir=.\release

; Require Admin rights to install (Crucial for VPN tools!)
PrivilegesRequired=admin
Compression=lzma
SolidCompression=yes
WizardStyle=modern

[InstallDelete]
; WIPE THE FOLDER BEFORE INSTALLING:
; This deletes all existing files and subdirectories ensuring a perfectly clean upgrade.
Type: filesandordirs; Name: "{app}\*"

[Tasks]
; The Checkboxes the user sees during installation
Name: "startmenuicon"; Description: "Create a Start Menu shortcut"; GroupDescription: "Shortcuts:"
Name: "desktopicon"; Description: "Create a Desktop shortcut"; GroupDescription: "Shortcuts:"; Flags: unchecked
Name: "envPath"; Description: "Add 'claviger' to system PATH (Allows running CLI from terminal)"; GroupDescription: "Advanced:"
Name: "startuponlogin"; Description: "Launch Claviger GUI automatically when Windows starts"; GroupDescription: "System Integration:"

[Registry]
; This adds the GUI to the Windows Startup Registry (Runs as Standard User). 
; The 'uninsdeletevalue' flag ensures it cleans up after itself if uninstalled!
Root: HKCU; Subkey: "SOFTWARE\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "ClavigerGateway"; ValueData: """{app}\claviger-windows-gui.exe"""; Flags: uninsdeletevalue; Tasks: startuponlogin

[Files]
; 1. Copy the GUI
Source: "build\claviger-windows-gui.exe"; DestDir: "{app}"; Flags: ignoreversion
; 2. Copy the CLI and rename it simply to "claviger.exe"
Source: "build\claviger-windows-cli.exe"; DestDir: "{app}"; DestName: "claviger.exe"; Flags: ignoreversion
; 3. Copy Wintun.dll (Required for WireGuard on Windows)
Source: "wintun.dll"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
; These are tied directly to the checkboxes in the [Tasks] section
Name: "{autoprograms}\Claviger Client"; Filename: "{app}\claviger-windows-gui.exe"; Tasks: startmenuicon
Name: "{autodesktop}\Claviger Client"; Filename: "{app}\claviger-windows-gui.exe"; Tasks: desktopicon

[Run]
; 1. Register the CLI as a native Windows Service to run as SYSTEM automatically on boot
Filename: "{sys}\sc.exe"; Parameters: "create ClavigerService binPath= ""\""{app}\claviger.exe\"" daemon"" start= auto"; Flags: runhidden; StatusMsg: "Registering Zero Trust Daemon..."

; 2. Start the service immediately so the tunnel engine is ready when the GUI opens
Filename: "{sys}\sc.exe"; Parameters: "start ClavigerService"; Flags: runhidden; StatusMsg: "Booting Background Engine..."

[UninstallRun]
; 1. Stop the service gracefully before wiping files during uninstall
Filename: "{sys}\sc.exe"; Parameters: "stop ClavigerService"; Flags: runhidden; RunOnceId: "StopClavigerService"

; 2. Delete the service from the Windows Registry to prevent ghost adapters
Filename: "{sys}\sc.exe"; Parameters: "delete ClavigerService"; Flags: runhidden; RunOnceId: "DeleteClavigerService"

[Code]
// --- MAGIC CODE TO ADD APP TO WINDOWS PATH AND HANDLE SAFE UPGRADES ---
const EnvironmentKey = 'SYSTEM\CurrentControlSet\Control\Session Manager\Environment';

procedure CurStepChanged(CurStep: TSetupStep);
var
  Paths: string;
  ResultCode: Integer;
begin
  // --- ZERO TRUST FIX: Safe Upgrades ---
  // If the user is upgrading, the Daemon is likely running. We MUST stop it 
  // right before files are copied, otherwise Windows will block the overwrite.
  if CurStep = ssInstall then
  begin
    Exec(ExpandConstant('{sys}\sc.exe'), 'stop ClavigerService', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;

  // --- PATH INJECTION ---
  if (CurStep = ssPostInstall) and IsTaskSelected('envPath') then
  begin
    if RegQueryStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths) then
    begin
      // Only add to PATH if it isn't already there
      if Pos(';' + ExpandConstant('{app}'), ';' + Paths + ';') = 0 then
      begin
        Paths := Paths + ';' + ExpandConstant('{app}');
        RegWriteStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths);
      end;
    end;
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  Paths: string;
  P: Integer;
begin
  if CurUninstallStep = usPostUninstall then
  begin
    if RegQueryStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths) then
    begin
      P := Pos(';' + ExpandConstant('{app}'), Paths);
      if P > 0 then
      begin
        Delete(Paths, P, Length(';' + ExpandConstant('{app}')));
        RegWriteStringValue(HKEY_LOCAL_MACHINE, EnvironmentKey, 'Path', Paths);
      end;
    end;
  end;
end;
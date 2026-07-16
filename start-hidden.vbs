' Start 3xui-lite in background (no console window)
Set sh = CreateObject("WScript.Shell")
dir = CreateObject("Scripting.FileSystemObject").GetParentFolderName(WScript.ScriptFullName)

' Skip if already listening on 18080
Set exec = sh.Exec("cmd /c netstat -ano | findstr :18080 | findstr LISTENING")
Do While exec.Status = 0
  WScript.Sleep 50
Loop
out = exec.StdOut.ReadAll
If InStr(out, "LISTENING") > 0 Then
  sh.Run "http://127.0.0.1:18080/", 1, False
  WScript.Quit 0
End If

sh.CurrentDirectory = dir
sh.Environment("Process")("XUI_LISTEN") = "127.0.0.1:18080"
sh.Environment("Process")("XUI_DATA") = dir & "\data"
sh.Environment("Process")("XRAY_BIN") = dir & "\bin\xray.exe"
sh.Environment("Process")("SINGBOX_BIN") = dir & "\bin\sing-box.exe"
' 0 = hidden window
sh.Run """" & dir & "\3xui-lite.exe""", 0, False
WScript.Sleep 1500
sh.Run "http://127.0.0.1:18080/", 1, False

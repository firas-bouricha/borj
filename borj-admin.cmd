@echo off
rem Lance BORJ en administrateur (necessaire pour l'usure des disques).
rem Windows Terminal si present, sinon console classique.
where wt.exe >nul 2>nul
if %errorlevel%==0 (
    powershell -NoProfile -Command "Start-Process wt.exe -Verb RunAs -ArgumentList ('--maximized --title BORJ \"' + '%~dp0borj.exe' + '\"')"
) else (
    powershell -NoProfile -Command "Start-Process -FilePath '%~dp0borj.exe' -Verb RunAs"
)

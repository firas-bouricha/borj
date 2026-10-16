@echo off
rem Lance BORJ en administrateur (necessaire pour l'usure des disques).
rem Windows Terminal si present, sinon console classique.
rem Le chemin passe par une variable d'environnement : aucun risque si le dossier contient une apostrophe.
set "BORJ_EXE=%~dp0borj.exe"
where wt.exe >nul 2>nul
if %errorlevel%==0 (
    powershell -NoProfile -Command "Start-Process wt.exe -Verb RunAs -ArgumentList @('--maximized', '--title', 'BORJ', ('\"' + $env:BORJ_EXE + '\"'))"
) else (
    powershell -NoProfile -Command "Start-Process -FilePath $env:BORJ_EXE -Verb RunAs"
)

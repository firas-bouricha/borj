@echo off
rem Lance BORJ dans Windows Terminal en administrateur (necessaire pour l'usure du SSD).
powershell -NoProfile -Command "Start-Process wt.exe -Verb RunAs -ArgumentList ('--maximized --title BORJ \"' + '%~dp0borj.exe' + '\"')"

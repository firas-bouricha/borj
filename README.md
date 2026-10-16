# BORJ

> La tour de guet de ton PC Windows.

**BORJ** est un tableau de bord dans le terminal pour Windows. Il réunit sur un seul écran la RAM, le CPU, le réseau, le disque, la santé du matériel, les machines virtuelles VirtualBox, les alertes et l'historique.

Le nom vient du *borj* (برج) : les tours de garde qui surveillaient les côtes et les routes de Tunisie. BORJ fait la même chose pour ta machine.

![BORJ](docs/screenshot.png)

## Pourquoi

Sur un portable avec 16 Go de RAM, Teams, VS Code, un navigateur, Docker et deux ou trois VMs suffisent à tout faire planter. Le Gestionnaire des tâches ne dit pas *qui* consomme vraiment, ni *quand* ça va casser. BORJ répond à ces deux questions :

- **Qui consomme ?** Les processus sont regroupés par application. Brave, avec ses 16 processus, apparaît sur une seule ligne, et Teams inclut ses processus WebView2.
- **Quand ça casse ?** BORJ suit le *commit charge* (RAM + fichier d'échange), la vraie limite avant les plantages, et envoie une notification Windows avant qu'il soit trop tard.

## Fonctionnalités

| | |
|---|---|
| **Temps réel** | CPU total et par cœur, RAM, commit, mémoire compressée, réseau, débits disque |
| **Applications** | Regroupement par appli, détail par processus, recherche, tri RAM / CPU / disque / nom |
| **Actions** | Arrêter une appli (avec confirmation, processus système protégés), *mode éco* (EcoQoS + priorité basse) |
| **Profil labo** | Une touche passe Teams, Discord, Brave, Edge… en mode éco avant de lancer les VMs |
| **VirtualBox** | Liste des VMs, démarrage sans fenêtre, pause, sauvegarde, arrêt ACPI, ballon mémoire |
| **Santé** | SSD (statut, usure, erreurs), CPU (erreurs WHEA), RAM (barrettes, slots libres, capacité max), avec un verdict : bonne santé / à surveiller / à remplacer |
| **Alertes** | Notification Windows quand la RAM ou le commit dépasse 95 % |
| **Historique** | CSV toutes les 5 s et journal des pics ≥ 85 % avec les applis responsables |

## Installation

Prérequis : Windows 10/11, [Go 1.26+](https://go.dev/dl/).

```bash
git clone https://github.com/firas-bouricha/borj.git
cd borj
go build -ldflags "-s -w" -o borj.exe .
```

## Utilisation

```bash
borj.exe
```

Pour voir l'usure du SSD, lance BORJ en administrateur : `borj-admin.cmd`.

| Touche | Action |
|---|---|
| `Tab` / `1` `2` `3` | Onglets : Applications, Machines virtuelles, Historique |
| `↑` `↓` `Entrée` | Naviguer, déplier une application |
| `/` | Rechercher (nom ou PID) |
| `m` `c` `d` `n` | Trier par RAM, CPU, disque, nom |
| `x` | Arrêter l'application sélectionnée |
| `e` | Mode éco sur l'application |
| `L` | Profil labo |
| `s` `p` `v` `a` `+` `-` | VMs : démarrer, pause, sauvegarder, éteindre, ballon mémoire |
| `r` | Relancer l'analyse santé |
| `q` | Quitter |

Les touches de l'onglet ouvert sont aussi expliquées dans la colonne de droite.

Options : `-nocolor` désactive les couleurs, `-once` affiche une seule image puis quitte.

## Réglages système

En attendant leur intégration dans BORJ, le dossier `scripts/` contient :

| Fichier | Rôle |
|---|---|
| `optimize-memory.ps1` | Réglages faits une seule fois, en administrateur : fusion de pages mémoire, fichier d'échange de taille fixe, SQL Server en démarrage manuel. Simulation par défaut, `-Apply` pour appliquer |
| `vscode/` | Réglages VS Code pour réduire sa consommation |

## Architecture

```
main.go         boucle de l'interface (bubbletea), touches, onglets
sys_windows.go  collecte : NtQuerySystemInformation, GetPerformanceInfo, compteurs réseau
health.go       santé SSD / CPU / RAM (Storage + CIM, toutes les 5 min)
vbox.go         pilotage de VBoxManage
alerts.go       seuils et notifications Windows
history.go      CSV et journal des pics
ui.go           rendu (lipgloss)
```

Les processus sont lus en **un seul appel système** (`NtQuerySystemInformation`, classe `SystemProcessInformation`), sans WMI ni `OpenProcess`. BORJ consomme ainsi environ 15 Mo de RAM.

## Feuille de route

- [ ] Graphiques haute résolution (braille) par cœur CPU
- [ ] Courbe du pagefile et occupation de tous les disques
- [ ] Section WSL et Docker
- [ ] Connexions réseau par application
- [ ] Mode arrière-plan (historique et alertes sans interface)
- [ ] Onglet Optimisation (remplace `optimize-memory.ps1`)

## Licence

[MIT](LICENSE) © 2026 Firas Bouricha

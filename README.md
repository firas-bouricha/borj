<div align="center">

# BORJ

**La tour de guet de ton PC Windows.**

[![CI](https://github.com/firas-bouricha/borj/actions/workflows/ci.yml/badge.svg)](https://github.com/firas-bouricha/borj/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/firas-bouricha/borj?include_prereleases&label=version)](https://github.com/firas-bouricha/borj/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/firas-bouricha/borj?logo=go)](go.mod)
[![Windows](https://img.shields.io/badge/Windows-10%20%7C%2011-0078D6?logo=windows)](#limites-actuelles)
[![Interface](https://img.shields.io/badge/interface-fran%C3%A7ais-blue)](#limites-actuelles)
[![Licence](https://img.shields.io/github/license/firas-bouricha/borj)](LICENSE)

</div>

**BORJ** est un tableau de bord dans le terminal pour Windows. Il réunit sur un seul écran la RAM, le CPU, le réseau, les disques, la santé du matériel, les machines virtuelles VirtualBox, les alertes et l'historique.

Le nom vient du *borj* (برج) : les tours de garde qui surveillaient les côtes et les routes de Tunisie. BORJ fait la même chose pour ta machine.

<!-- Capture d'écran : docs/screenshot.png -->

---

## Sommaire

- [Pourquoi](#pourquoi)
- [Fonctionnalités](#fonctionnalités)
- [Installation](#installation)
- [Lancer BORJ](#lancer-borj)
- [Utiliser les fonctionnalités](#utiliser-les-fonctionnalités)
- [Limites actuelles](#limites-actuelles)
- [Technologies](#technologies)
- [Feuille de route](#feuille-de-route)

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
| **Santé** | Tous les disques (statut, usure, erreurs), CPU (erreurs matérielles WHEA), RAM (barrettes, slots libres, capacité max), avec un verdict : bonne santé / à surveiller / à remplacer |
| **Volumes** | Occupation de chaque lecteur (C:, D:, USB, réseau) : orange au-dessus de 85 %, rouge au-dessus de 95 % |
| **Réseau** | IP privées par carte, IP publique à la demande (masquée), ports ouverts par application (exposé / local), connexions actives |
| **Alertes** | Notification Windows quand la RAM ou le commit dépasse 95 % |
| **Historique** | CSV toutes les 5 s et journal des pics ≥ 85 % avec les applis responsables |

## Installation

### Option 1 : télécharger l'exécutable (recommandé)

1. Va sur la page [**Releases**](https://github.com/firas-bouricha/borj/releases/latest) et télécharge `borj-windows-amd64.zip`.
2. Fais un clic droit sur le zip → **Extraire tout**, par exemple dans `C:\Outils\borj`.
3. Double-clique sur `borj.exe`.

> **« Windows a protégé votre ordinateur »** : l'exécutable n'est pas signé (la signature de code est payante), donc SmartScreen l'affiche au premier lancement. Clique sur **Informations complémentaires**, puis **Exécuter quand même**. Le code source est entièrement disponible ici, et tu peux vérifier le fichier avec `checksums.txt` (SHA-256).

**Optionnel :** pour lancer `borj` depuis n'importe quel terminal, ajoute son dossier au `PATH` :

```powershell
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";C:\Outils\borj", "User")
```

### Option 2 : compiler depuis les sources

Prérequis : [Go 1.26.9+](https://go.dev/dl/).

```bash
git clone https://github.com/firas-bouricha/borj.git
cd borj
go build -ldflags "-s -w" -o borj.exe .
```

## Lancer BORJ

| Méthode | Quand l'utiliser |
|---|---|
| Double-clic sur `borj.exe` | Usage normal |
| Double-clic sur `borj-admin.cmd` | Pour voir **l'usure des disques** (demande les droits administrateur). Ouvre Windows Terminal en plein écran s'il est installé, sinon la console classique |
| `borj` dans un terminal | Si son dossier est dans le `PATH` |

Options de la ligne de commande :

| Option | Effet |
|---|---|
| `-version` | Affiche la version |
| `-nocolor` | Désactive les couleurs |
| `-once` | Affiche une seule image puis quitte (pratique pour un rapport ou un test) |

Conseil : utilise **Windows Terminal** en plein écran. L'affichage s'adapte à la taille de la fenêtre, et la colonne des commandes apparaît à partir de 150 colonnes de large.

## Utiliser les fonctionnalités

BORJ a quatre onglets : `Tab` passe de l'un à l'autre, ou utilise `1`, `2`, `3`, `4`. La colonne de droite rappelle toujours les touches de l'onglet ouvert.

### Partout

| Touche | Action |
|---|---|
| `Tab` / `1` `2` `3` `4` | Changer d'onglet |
| `L` | **Profil labo** : met Teams, Discord, Brave, Edge, OneDrive et Google Drive en mode éco. À utiliser avant de lancer des VMs. Appuie à nouveau pour annuler |
| `r` | Relancer l'analyse santé (faite automatiquement toutes les 5 min) |
| `q` | Quitter |

### 1 · Applications

| Touche | Action |
|---|---|
| `↑` `↓` | Choisir une application |
| `Entrée` | Déplier ou replier : voir chaque processus avec son PID |
| `/` | Rechercher un nom ou un PID (`Échap` pour effacer) |
| `m` `c` `d` `n` | Trier par RAM, CPU, disque ou nom |
| `x` | Arrêter l'application ou le processus (confirmation `o` / `n`). Les processus système sont protégés |
| `e` | **Mode éco** 🍃 : priorité basse + EcoQoS. Windows bride l'application et libère le CPU. Le réglage reste actif même quand l'application relance des processus |

### 2 · Machines virtuelles (VirtualBox)

| Touche | Action |
|---|---|
| `↑` `↓` | Choisir une VM |
| `s` | Démarrer sans fenêtre (consomme moins) |
| `p` | Pause ou reprise : gèle la VM et libère le CPU |
| `v` | Sauvegarder l'état : la VM est figée sur le disque et **toute sa RAM est libérée** |
| `a` | Éteindre proprement (confirmation) |
| `+` `-` | Ballon mémoire : reprend ou rend 512 Mo à la VM (*Guest Additions* requises) |
| `u` | Actualiser la liste |

Le résumé en haut indique la RAM réservée par les VMs allumées et le nombre de vCPU par rapport aux cœurs réels.

### 3 · Historique

- Courbes RAM, commit et CPU sur la dernière heure.
- Liste des **pics ≥ 85 %**, avec l'heure et les applications responsables.
- `o` ouvre le dossier des journaux : `%LOCALAPPDATA%\Borj` (un CSV par jour et `pics.log`).

### 4 · Réseau

| Touche | Action |
|---|---|
| `i` | Demander l'**IP publique** à Cloudflare (`1.1.1.1`). C'est la seule requête Internet de BORJ, et elle ne part que sur cette touche |
| `v` | Afficher ou masquer l'IP publique (masquée par défaut : `85.169.•••.•••`) |
| `↑` `↓` | Faire défiler les ports ouverts |

- **Mes adresses** : IP privée de chaque carte avec un nom clair (Wi-Fi, Ethernet, VirtualBox, WSL / Docker, VMware, Tailscale, VPN) et la passerelle.
- **Ports ouverts** : chaque port en écoute, son service (SSH, SMB, PostgreSQL…) et l'application propriétaire. ● rouge = **exposé** (écoute sur le réseau ; le pare-feu Windows peut encore bloquer), ● vert = **local uniquement**.
- **Connexions actives** : connexions établies regroupées par application, avec leurs adresses distantes principales.

### Alertes

Quand la RAM ou le commit dépasse **95 %**, BORJ envoie une notification Windows et affiche un bandeau rouge. L'alerte se réarme quand la valeur redescend sous 90 %.

### Réglages système

En attendant leur intégration dans BORJ, le dossier `scripts/` contient :

| Fichier | Rôle |
|---|---|
| `optimize-memory.ps1` | Réglages faits une seule fois, en administrateur : fusion de pages mémoire, fichier d'échange de taille fixe, SQL Server en démarrage manuel. Simulation par défaut, `-Apply` pour appliquer |
| `vscode/` | Réglages VS Code pour réduire sa consommation |

## Limites actuelles

| Limite | Détail |
|---|---|
| **Windows uniquement** | Windows 10 (1709 et plus) et 11, en 64 bits. Pas de Linux ni de macOS : BORJ utilise directement les API de Windows |
| **Interface en français** | Pas encore de version anglaise |
| **Usure des disques en admin** | Sans droits administrateur, Windows ne fournit pas l'usure ni les erreurs des disques |
| **Santé via PowerShell** | L'analyse santé et les notifications passent par PowerShell. Sur un PC d'entreprise qui le bloque, ces deux parties sont indisponibles. Le reste fonctionne |
| **VirtualBox seulement** | Hyper-V, VMware, WSL et Docker ne sont pas encore pilotés (WSL apparaît dans la liste des applications) |
| **Historique et alertes** | Ils ne fonctionnent que lorsque BORJ est ouvert |
| **Pas de températures** | Windows ne les expose pas de façon standard sur la plupart des portables |
| **Exécutable non signé** | Avertissement SmartScreen au premier lancement (voir [Installation](#installation)) |

## Sécurité

BORJ est conçu pour **observer** la machine, pas pour la modifier.

**Ce que BORJ ne fait jamais**
- Aucune écriture dans le registre, les dossiers système ou les fichiers des applications
- Aucune connexion réseau automatique : pas de télémétrie, pas de mise à jour. Seule exception, l'IP publique, demandée à Cloudflare (`1.1.1.1`) **uniquement** quand tu appuies sur `i`
- Aucun service ni pilote installé, rien qui se lance au démarrage

**Seul dossier écrit** : `%LOCALAPPDATA%\Borj` (historique CSV et journal des pics). Les CSV de plus de 30 jours sont supprimés automatiquement.

**Actions possibles, toutes déclenchées par l'utilisateur**

| Action | Garde-fous |
|---|---|
| Arrêter un processus (`x`) | Confirmation obligatoire. Processus vitaux de Windows refusés (lsass, csrss, winlogon, services, svchost, Defender, Windows Update, installeur MSI…). Processus de VM refusés : on passe par l'onglet VMs. Vérification de l'heure de création : un numéro de processus (PID) réutilisé par un autre programme n'est jamais visé |
| Mode éco (`e`, `L`) | Change seulement la priorité et le mode EcoQoS. La priorité d'origine est rendue en quittant BORJ |
| VMs (`s` `p` `v` `a` `+` `-`) | Passe par `VBoxManage`. L'arrêt demande une confirmation et reste propre (bouton ACPI) |

**Programmes lancés** : PowerShell (santé et notifications, via son chemin absolu dans `System32`), `VBoxManage` (uniquement depuis le dossier d'installation de VirtualBox) et l'Explorateur (touche `o`). Jamais via le `PATH`.

**Dépendances** : analysées avec [`govulncheck`](https://go.dev/doc/security/vuln/). Les tests (`go test`) vérifient les protections ci-dessus à chaque push.

> Note : l'analyse santé passe par `powershell -EncodedCommand`, une forme que certains antivirus d'entreprise surveillent. C'est volontaire : le script est intégré au programme et n'est jamais écrit sur le disque, donc aucun autre programme ne peut le modifier.

## Technologies

| | |
|---|---|
| **Langage** | [Go](https://go.dev) |
| **Interface** | [Bubble Tea](https://github.com/charmbracelet/bubbletea) et [Lip Gloss](https://github.com/charmbracelet/lipgloss) (Charm) |
| **Système** | `golang.org/x/sys/windows` : `NtQuerySystemInformation`, `GetPerformanceInfo`, `GetIfEntry2Ex`, `SetProcessInformation` (EcoQoS) |
| **Santé** | PowerShell (Storage, CIM, journaux d'événements WHEA) |
| **VMs** | `VBoxManage` |
| **CI / Release** | GitHub Actions |

Les processus sont lus en **un seul appel système** (`NtQuerySystemInformation`, classe `SystemProcessInformation`), sans WMI ni `OpenProcess`. BORJ consomme environ 15 Mo de RAM.

```
main.go         boucle de l'interface, touches, onglets
sys_windows.go  collecte système (processus, CPU, mémoire, réseau) et actions
health.go       santé disques / CPU / RAM
vbox.go         pilotage de VirtualBox
alerts.go       seuils et notifications Windows
history.go      CSV et journal des pics
net.go          réseau : adresses, ports, connexions, IP publique
ui.go           rendu (ui_net.go : onglet Réseau)
```

## Feuille de route

- [ ] Graphiques haute résolution (braille) par cœur CPU
- [x] Occupation de tous les volumes
- [ ] Courbe du pagefile
- [ ] Section WSL et Docker
- [x] Onglet Réseau : adresses, ports ouverts, connexions par application
- [ ] Mode arrière-plan (historique et alertes sans interface)
- [ ] Onglet Optimisation (remplace `optimize-memory.ps1`)
- [ ] Interface en anglais

## Licence

[MIT](LICENSE) © 2026 Firas Bouricha

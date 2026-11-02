# Roborock Bridge pour Loxone — v1.0.0

La passerelle est désormais centrée sur une seule utilisation : retrouver et
piloter vos robots Roborock dans Loxone.

## Une interface plus simple

- **Mes robots** : une vue claire de tous les robots, de leur batterie et des
  éventuelles alertes. Les programmes Roborock restent accessibles.
- **Loxone** : choisissez un robot, retrouvez ses noms d’entrées exacts et copiez
  ses commandes. Les diagnostics et les options techniques sont rangés à part.
- **Mises à jour** : recherchez puis installez une version lorsque vous le
  décidez, avec sauvegarde et retour arrière en cas d’échec.
- Interface principale en français, adaptée aux ordinateurs et aux téléphones.

## Votre installation existante est conservée

Gardez le même volume de données, la même adresse IP et le même port. Les noms
d’entrées virtuelles, les commandes HTTP, les identifiants permanents des robots,
les jetons, les personnalisations et les connexions MQTT restent compatibles.
Il n’est pas nécessaire de recréer les entrées de votre robot dans Loxone.
Les anciens plannings et les préférences de mise à jour existantes sont conservés.

Les commandes de la passerelle fonctionnent maintenant aussi en connexion
Loxone HTTP directe seule, sans le message « Loxone mode is disabled ».

## Plusieurs robots

Ajoutez les robots au même compte dans l’application Roborock, puis redémarrez
le conteneur du bridge. La liste du compte est récupérée au démarrage ; les
adresses des robots déjà installés restent inchangées.

## Installation et mise à jour

**Installation existante :** dans votre passerelle, ouvrez **Système & mises à
jour**, recherchez la version **Stable**, puis choisissez **Installer**. Vous
pouvez conserver votre projet Container Manager actuel.

**Nouvelle installation Synology :** utilisez le fichier `compose.synology.yaml`
ou le dossier d’installation joint à cette publication, puis suivez
`docs/INSTALLATION_SYNOLOGY.md`. Les images comprennent les architectures
Linux `amd64` et `arm64`.

Image : `ghcr.io/itashi37/roborock-mqtt-loxone:v1.0.0`.

## Vérifications avant publication

Tests serveur complets avec détection des accès concurrents, analyse statique,
tests frontend et compilation réussis. Douze scénarios navigateur sur la
compilation de production couvrent notamment deux robots, les adresses Loxone
existantes, les commandes, la première installation et l’affichage mobile.
Ces scénarios utilisent des robots simulés ; ils n’installent rien sur un NAS.

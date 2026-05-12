# Guide d'évaluation du projet

## Préliminaires

### Conditions de base

*   **Vérification du dépôt:**
    *   Pas de rendu (= rien dans le dépôt git)
    *   Pas de fichier auteur
    *   Fichier auteur invalide
    *   Faute de Norme (avec la norminette)
    *   Triche
    *   Fonctions interdites (sauf si justifié dans les bonus). Dans ce projet l'étudiant est libre de choisir le langage, les librairies externes sont autorisées uniquement pour le parsing de la configuration et pour faire les bonus clients/serveurs. Pour le reste il faut se limiter à la librairie standard.
*   **Stabilité du programme:** Pas de segfault, ou d'autre arrêt intempestif et prématuré et non contrôlé du programme.

## Tests basiques

### Control shell

*   Le programme fournit plusieurs commandes shell sympathique, soit dans le programme lui même ou dans un programme séparé (comme supervisorctl). Le shell permet de démarrer, arrêter, et redémarrer le programme.

### Fichier de configuration

*   Le programme charge sa configuration depuis le fichier de configuration, peut importe le format. Pour cette fonctionnalité, les librairies externes (comme la librairie de parsing YAML, par exemple) sont autorisées. Le programme décrit dans le fichier de configuration est actuellement chargé, ses statuts peuvent être consultés avec le shell, etc...

### Logging

*   Ceci est un système de log, qui se connecte à un fichier (Plus de point dans la section bonus). Pour que ce soit vrai, il doit y avoir un nombre raisonnable d'événements pris en considération: Quand les programmes sont démarrés, arrêtés, redémarrés, quand ils s'arrêtent de façon inattendue, Quand le projet aborts au lancement à cause d'un nombre trop important d'essais, etc...

### Hot-reload

*   La configuration peut être rechargée tant que le programme est lancé, à la fois avec un SIGHUP et avec une commande shell. Lorsque le reload prend effet, l'état du programme doit être modifié pour être conforme à sa nouvelle configuration. Cependant, les programmes qui ne sont pas affectés par le changement de configuration ne doivent en aucun cas être redémarrés (Ca serait très, très bâclé)

### Options de configuration

*   Quelle commande utiliser pour lancer le programme?
*   Combien de processus pour le démarrer et le garder lancé?
*   Est-ce que le programme peut automatiquement être lancé au démarrage ou non?
*   Vérifier si le programme redémarre tout le temps, jamais, ou seulement lors d'une sortie inattendue.
*   Quel code de retour représente une sortie "attendue"?
*   Combien de temps attendre pour que le programme nous dise "successfully started"?
*   Combien de fois il faut tenter de relancer le programme avant qu'il "abort"?
*   Quel signal doit être utilisé pour arrêter le programme proprement?
*   Combien de temps attendre après l'essai d'un "graceful stop" avant de tuer le programme?
*   Quel est l'option pour rediriger stdout/stderr dans un fichier ou pour s'en débarrasser?
*   Quelles variables d'environnement pour définir ce programme?
*   Quel est le répertoire de travail pour ce programme?
*   Umask pour ce programme

## Tests

*   **Stabilité du programme:** Est-ce que le programme reste debout après une variété de test?
*   **Scénarios de test:**
    *   Tuer un processus supervisé, et vérifiez qu'il redémarre automatiquement
    *   Essayer de superviser un processus qui peut uniquement sortir une erreur, et vérifiez qu'il abort après un certains nombre de tentatives
*   **Tests supplémentaires:** Vous pouvez ensuite essayer (raisonnablement) des tests que vous pensez qui peuvent faire planter le programme.

## Bonus

*   **Points:** Comptez 1 point pour chaque test correctement implémenté.
*   **Bonus suggérés:**
    *   De-escalation des privilèges au lancement (Autoriser un programme à ce lancer en tant qu'utilisateur spécifique)
    *   L'architecture Client/serveur comme supervisor (Le programme principal qui fait le travail est actuellement un daemon, et un programme séparé communique avec lui via unix/tcp sockets ou tout autre méthode pour fournir le contrôle du shell)
    *   Des installations de logging/reporting plus avancés (Email, http, syslog, whatever)
    *   Option pour "attach" un processus supervisé pour la console courante, à la manière de screen ou tmux (Compte pour 2 !). Évidemment, tout autres bonus que les étudiants décide d'implémenter compte.

## Notation

*   Rate it from 0 (failed) through 5 (excellent)
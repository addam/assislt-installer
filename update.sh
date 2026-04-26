#!/bin/bash
# harp web web-out;
rsync -r --info=progress2 server/{app.py,products.json,requirements.txt} dominec.eu:/var/www/artikulo-installer;
rm -r web-out;

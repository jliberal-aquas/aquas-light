# Gobierno y producto tienen linters y pruebas separados

El gobierno es código Go con sus linters fijos desde la instalación. El producto vive en `product/` y define los suyos según su lenguaje, por ejemplo FastAPI. Un cambio se valida con las herramientas del lado que toca, nunca con las del otro.

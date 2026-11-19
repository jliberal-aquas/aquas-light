# La documentación se sella contra lo que describe

Cada documento guarda en `work/seals.json` la huella de los archivos y especificaciones que cubre; si una cambia, el commit se bloquea hasta resellarlo. Nació del caso de la página F2, que acumuló once afirmaciones falsas porque el verificador comparaba la wiki con el plan y nunca con el código.

## Consequences

Cada bug obliga a revisar la cadena completa hasta la capacidad. Un resello sin cambios exige una razón escrita que el auditor lee al cerrar la feature.

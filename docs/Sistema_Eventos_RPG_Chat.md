# **Arquitectura: Sistema de Eventos RPG para Chat**

Este documento define la estructura lógica y operativa para implementar un sistema interactivo de tipo RPG en un chat de streaming, gestionando la economía y el progreso en una base de datos externa.

## **1\. Conceptos y Entidades Base**

Para optimizar el rendimiento, el sistema debe manejar el estado de los eventos activos en memoria y sincronizar con la base de datos externa al concluir.

* **Usuario (User):** Contiene un identificador único, nombre de usuario, saldo actual de puntos y la referencia al arma que tiene equipada actualmente (si no tiene, se asume un ataque base).  
* **Arma (Weapon):** Definida por un identificador, nombre, valor de daño que aporta y precio en puntos.  
* **Inventario (Inventory):** Tabla o relación que vincula qué armas posee cada usuario.  
* **Estado de Canal (Channel State):** Variable global que determina la actividad actual. Puede ser: ESPERA, MAZMORRA\_ACTIVA, JEFE\_ACTIVO o COFRE\_ACTIVO.

## **2\. El Motor de Eventos**

Un proceso en segundo plano (Event Loop) que dicta el ritmo del juego sin intervenir en la lectura constante del chat.

* **El Ciclo Temporizado:** Un temporizador interno evalúa periódicamente (ej. cada cierto número de minutos) si se debe desencadenar un evento.  
* **Control de Concurrencia:** Asegura que el estado del canal no cambie mientras se están calculando los resultados de un evento, bloqueando transiciones no deseadas.  
* **Motor de Probabilidad (RNG):** Al cumplirse el ciclo, se utiliza una distribución de probabilidad para elegir qué evento ocurrirá a continuación (ej. 50% Jefe, 30% Mazmorra, 20% Cofre).

## **3\. Módulos del Sistema**

### **Módulo A: Economía y Equipamiento**

Comandos pasivos que los espectadores pueden utilizar siempre que el canal esté en estado de ESPERA.

* **Comando Tienda:** El bot responde con el catálogo actual de armas y sus precios.  
* **Comando Comprar:** 1\. Verifica si el usuario tiene puntos suficientes.  
  2\. Descuenta los puntos de su cuenta.  
  3\. Registra el arma en su inventario.  
* **Comando Equipar:** Actualiza el registro del usuario indicando cuál de las armas de su inventario usará en el próximo combate.

### **Módulo B: La Mazmorra (Incursión)**

Evento cooperativo que evalúa el poder total acumulado de los participantes frente a un obstáculo predeterminado.

1. **Activación:** El sistema entra en estado MAZMORRA\_ACTIVA. Se genera y oculta un valor de "Resistencia de Mazmorra" aleatorio. Se inicia una cuenta regresiva.  
2. **Inscripción:** Los usuarios escriben el comando para entrar. El sistema los añade a una lista temporal en memoria junto con el valor de daño de su arma equipada. Se evita la doble inscripción.  
3. **Resolución:** Al expirar el tiempo, finalizan las inscripciones.  
   * Se suma el daño de todos los participantes para obtener el "Poder Total de la Party".  
   * **Evaluación:**  
     * Si Poder Total \>= Resistencia: Victoria. Se reparte una recompensa entre los participantes.  
     * Si Poder Total \< Resistencia: Derrota. Los participantes no ganan puntos (o se les aplica una penalización).  
   * **Sincronización:** Se realiza una escritura masiva a la base de datos actualizando los puntos de los involucrados. El estado vuelve a ESPERA.

### **Módulo C: El Jefe Errante**

Evento de "Daño por Segundo" (DPS) donde la cantidad y calidad de los ataques determina el éxito.

1. **Activación:** Entra en estado JEFE\_ACTIVO. Se define la "Vida Total" del jefe y se inicializa un registro en memoria de los atacantes y el daño que han causado.  
2. **Combate:** Los usuarios escriben el comando de ataque.  
   * El sistema identifica el arma del usuario.  
   * **Cálculo de Daño:** Daño del Arma \+ una ligera variación aleatoria (ej. \+/- 10%).  
   * Se resta este daño a la Vida Total del jefe.  
   * Se acumula el daño infligido en el registro personal de ese usuario.  
3. **Resolución:** Cuando la vida del jefe llega a 0\.  
   * El sistema deja de registrar ataques.  
   * Se define una bolsa total de puntos como recompensa.  
   * A cada participante se le otorga un porcentaje de la bolsa proporcional al daño que causaron al jefe.  
   * **Sincronización:** Escritura masiva a la base de datos con los balances finales. Retorno al estado ESPERA.

### **Módulo D: El Cofre Solitario**

Un evento rápido que evalúa los reflejos y el factor riesgo/recompensa.

1. **Activación:** El estado cambia a COFRE\_ACTIVO y se notifica la aparición del cofre en el chat.  
2. **Interacción:** Los usuarios intentan abrirlo.  
   * El sistema procesa **únicamente** la primera solicitud válida que reciba.  
   * Inmediatamente tras recibirla, el estado cambia a ESPERA, ignorando los intentos posteriores.  
3. **Resolución (RNG):**  
   * Porcentaje Alto (Ej. 70%): Es un tesoro. Se suman puntos al primer usuario.  
   * Porcentaje Bajo (Ej. 30%): Es un mímico (trampa). Se le restan puntos al primer usuario.  
   * **Sincronización:** Actualización directa del balance del usuario en la base de datos.

## **4\. Flujo de Procesamiento de Mensajes**

Para garantizar la fluidez del chat, especialmente durante eventos de alta actividad, el procesamiento de comandos debe seguir un embudo estricto:

1. **Recepción:** El bot escucha todos los mensajes que entran al chat.  
2. **Filtro Inicial:** Solo los mensajes que comienzan con el prefijo de comando (ej. \!) pasan a la siguiente etapa.  
3. **Enrutamiento y Validación de Estado:**  
   * Comandos pasivos (inventario, tienda) se procesan directamente.  
   * Comandos de evento (atacar, entrar, abrir) verifican el Estado de Canal. Si el estado no coincide con el comando (ej. intentar atacar cuando no hay jefe), el mensaje se descarta inmediatamente antes de hacer cualquier consulta a la base de datos externa.
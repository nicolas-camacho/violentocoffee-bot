#include <3ds.h>
#include <stdio.h>
#include <stdlib.h>
#include <malloc.h>
#include <string.h>
#include <arpa/inet.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include <fcntl.h>
#include <unistd.h>

#define SOC_ALIGN       0x1000
#define SOC_BUFFERSIZE  0x100000
#define UDP_PORT        8888
#define MAX_MESSAGES    18
#define MSG_MAX_LEN     128

static u32 *soc_buffer = NULL;
static char messages[MAX_MESSAGES][MSG_MAX_LEN];
static int msg_count = 0;

static void push_message(const char *msg) {
    if (msg_count < MAX_MESSAGES) {
        snprintf(messages[msg_count++], MSG_MAX_LEN, "%s", msg);
    } else {
        for (int i = 0; i < MAX_MESSAGES - 1; i++)
            memcpy(messages[i], messages[i + 1], MSG_MAX_LEN);
        snprintf(messages[MAX_MESSAGES - 1], MSG_MAX_LEN, "%s", msg);
    }
}

static void redraw_top(PrintConsole *top) {
    consoleSelect(top);
    consoleClear();
    printf("\x1b[0;0H-- ViolentoCoffee 3DS Messenger --\n\n");
    for (int i = 0; i < msg_count; i++)
        printf("  %s\n", messages[i]);
}

int main(void) {
    gfxInitDefault();

    PrintConsole top, bottom;
    consoleInit(GFX_TOP, &top);
    consoleInit(GFX_BOTTOM, &bottom);

    consoleSelect(&bottom);
    printf("\x1b[0;0HIniciando red...\n");
    gfxFlushBuffers();
    gfxSwapBuffers();

    soc_buffer = (u32 *)memalign(SOC_ALIGN, SOC_BUFFERSIZE);
    if (!soc_buffer) {
        printf("Sin memoria para SOC\n");
        goto wait_exit;
    }

    Result ret = socInit(soc_buffer, SOC_BUFFERSIZE);
    if (R_FAILED(ret)) {
        printf("Error SOC: 0x%08lX\n", ret);
        free(soc_buffer);
        soc_buffer = NULL;
        goto wait_exit;
    }

    int sock = socket(AF_INET, SOCK_DGRAM, IPPROTO_UDP);
    if (sock < 0) {
        printf("Error al crear socket\n");
        goto cleanup_soc;
    }

    // Modo no bloqueante para que el loop siga respondiendo a botones
    int fl = fcntl(sock, F_GETFL, 0);
    fcntl(sock, F_SETFL, fl | O_NONBLOCK);

    struct sockaddr_in srv;
    memset(&srv, 0, sizeof(srv));
    srv.sin_family      = AF_INET;
    srv.sin_port        = htons(UDP_PORT);
    srv.sin_addr.s_addr = INADDR_ANY;

    if (bind(sock, (struct sockaddr *)&srv, sizeof(srv)) < 0) {
        printf("Error al hacer bind en puerto %d\n", UDP_PORT);
        close(sock);
        goto cleanup_soc;
    }

    struct in_addr ip_addr;
    ip_addr.s_addr = gethostid();

    consoleSelect(&bottom);
    consoleClear();
    printf("\x1b[0;0H[ViolentoCoffee 3DS]\n\n");
    printf("IP  : %s\n", inet_ntoa(ip_addr));
    printf("Port: %d\n\n", UDP_PORT);
    printf("Esperando mensajes...\n\n");
    printf("[START] para salir\n");

    redraw_top(&top);
    gfxFlushBuffers();
    gfxSwapBuffers();

    char buf[MSG_MAX_LEN];
    bool needs_redraw = false;
    struct sockaddr_in sender;
    socklen_t sender_len = sizeof(sender);

    while (aptMainLoop()) {
        hidScanInput();
        if (hidKeysDown() & KEY_START) break;

        ssize_t n = recvfrom(sock, buf, sizeof(buf) - 1, 0,
                             (struct sockaddr *)&sender, &sender_len);
        if (n > 0) {
            buf[n] = '\0';
            sendto(sock, "OK", 2, 0, (struct sockaddr *)&sender, sender_len);
            push_message(buf);
            needs_redraw = true;
        }

        if (needs_redraw) {
            redraw_top(&top);
            needs_redraw = false;
        }

        gfxFlushBuffers();
        gfxSwapBuffers();
        gspWaitForVBlank();
    }

    close(sock);

cleanup_soc:
    socExit();
    if (soc_buffer) free(soc_buffer);
    gfxExit();
    return 0;

wait_exit:
    gfxFlushBuffers();
    gfxSwapBuffers();
    while (aptMainLoop()) {
        hidScanInput();
        if (hidKeysDown() & KEY_START) break;
        gspWaitForVBlank();
    }
    gfxExit();
    return 1;
}

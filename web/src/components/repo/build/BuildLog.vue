<template>
  <div v-if="build" class="flex flex-col pt-10 md:pt-0">
    <div
      class="fixed top-0 left-0 w-full md:hidden flex px-4 py-2 bg-gray-600 dark:bg-dark-gray-800 text-gray-50"
      @click="$emit('update:proc-id', null)"
    >
      <span>{{ proc?.name }}</span>
      <Icon name="close" class="ml-auto" />
    </div>

    <div
      class="flex flex-grow flex-col bg-gray-300 dark:bg-dark-gray-700 md:m-2 md:mt-0 md:rounded-md overflow-hidden"
      @mouseover="showActions = true"
      @mouseleave="showActions = false"
    >
      <div v-show="showActions" class="absolute top-0 right-0 z-50 mt-2 mr-4 hidden md:flex">
        <Button
          v-if="proc?.end_time !== undefined"
          :is-loading="downloadInProgress"
          :title="$t('repo.build.actions.log_download')"
          start-icon="download"
          @click="download"
        />
      </div>

      <div
        v-show="hasLogs && loadedLogs"
        ref="consoleElement"
        class="w-full max-w-full grid grid-cols-[min-content,1fr,min-content] auto-rows-min flex-grow p-2 gap-x-2 overflow-x-hidden overflow-y-auto"
      >
        <div v-for="line in log" :key="line.index" class="contents font-mono">
          <span class="text-gray-500 whitespace-nowrap select-none text-right">{{ line.index + 1 }}</span>
          <!-- eslint-disable-next-line vue/no-v-html -->
          <span class="align-top text-color whitespace-pre-wrap break-words" v-html="line.text" />
          <span class="text-gray-500 whitespace-nowrap select-none text-right">{{ formatTime(line.time) }}</span>
        </div>
      </div>

      <div class="m-auto text-xl text-color">
        <span v-if="proc?.error" class="text-red-400">{{ proc.error }}</span>
        <span v-else-if="proc?.state === 'skipped'" class="text-red-400">{{ $t('repo.build.actions.canceled') }}</span>
        <span v-else-if="!proc?.start_time">{{ $t('repo.build.step_not_started') }}</span>
        <div v-else-if="!loadedLogs">{{ $t('repo.build.loading') }}</div>
      </div>

      <div
        v-if="proc?.end_time !== undefined"
        :class="proc.exit_code == 0 ? 'dark:text-lime-400 text-lime-700' : 'dark:text-red-400 text-red-600'"
        class="w-full bg-gray-200 dark:bg-dark-gray-800 text-md p-4"
      >
        {{ $t('repo.build.exit_code', { exitCode: proc.exit_code }) }}
      </div>
    </div>
  </div>
</template>

<script lang="ts">
import '~/style/console.css';

import AnsiUp from 'ansi_up';
import { debounce } from 'lodash';
import { computed, defineComponent, inject, nextTick, onMounted, PropType, Ref, ref, toRef, watch } from 'vue';
import { useI18n } from 'vue-i18n';

import Button from '~/components/atomic/Button.vue';
import Icon from '~/components/atomic/Icon.vue';
import useApiClient from '~/compositions/useApiClient';
import useNotifications from '~/compositions/useNotifications';
import { Build, Repo } from '~/lib/api/types';
import { findProc, isProcFinished, isProcRunning } from '~/utils/helpers';

type LogLine = {
  index: number;
  text: string;
  time?: number;
};

export default defineComponent({
  name: 'BuildLog',

  components: { Icon, Button },

  props: {
    build: {
      type: Object as PropType<Build>,
      required: true,
    },

    // used by toRef
    // eslint-disable-next-line vue/no-unused-properties
    procId: {
      type: Number,
      required: true,
    },
  },

  emits: {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    'update:proc-id': (procId: number | null) => true,
  },

  setup(props) {
    const notifications = useNotifications();
    const i18n = useI18n();
    const build = toRef(props, 'build');
    const procId = toRef(props, 'procId');
    const repo = inject<Ref<Repo>>('repo');
    const apiClient = useApiClient();

    const loadedProcSlug = ref<string>();
    const procSlug = computed(() => `${repo?.value.owner} - ${repo?.value.name} - ${build.value.id} - ${procId.value}`);
    const proc = computed(() => build.value && findProc(build.value.procs || [], procId.value));
    const stream = ref<EventSource>();
    const log = ref<LogLine[]>();
    const consoleElement = ref<Element>();

    const loadedLogs = computed(() => !!log.value);
    const hasLogs = computed(
      () =>
        // we do not have logs for skipped jobs
        repo?.value && build.value && proc.value && proc.value.state !== 'skipped' && proc.value.state !== 'killed',
    );
    const autoScroll = ref(true); // TODO: allow enable / disable
    const showActions = ref(false);
    const downloadInProgress = ref(false);
    const ansiUp = ref(new AnsiUp());
    ansiUp.value.use_classes = true;
    const logBuffer = ref<LogLine[]>([]);

    const maxLineCount = 500; // TODO: think about way to support lazy-loading more than last 300 logs (#776)

    function formatTime(time?: number): string {
      return time === undefined ? '' : `${time}s`;
    }

    function writeLog(line: LogLine) {
      logBuffer.value.push({
        index: line.index ?? 0,
        text: ansiUp.value.ansi_to_html(line.text),
        time: line.time ?? 0,
      });
    }

    function scrollDown() {
      nextTick(() => {
        if (!consoleElement.value) {
          return;
        }
        consoleElement.value.scrollTop = consoleElement.value.scrollHeight;
      });
    }

    const flushLogs = debounce((scroll: boolean) => {
      let buffer = logBuffer.value.slice(-maxLineCount);
      logBuffer.value = [];

      if (buffer.length === 0) {
        if (!log.value) {
          log.value = [];
        }
        return;
      }

      // append old logs lines
      if (buffer.length < maxLineCount && log.value) {
        buffer = [...log.value.slice(-(maxLineCount - buffer.length)), ...buffer];
      }

      // deduplicate repeating times
      buffer = buffer.reduce(
        (acc, line) => ({
          lastTime: line.time ?? 0,
          lines: [
            ...acc.lines,
            {
              ...line,
              time: acc.lastTime === line.time ? undefined : line.time,
            },
          ],
        }),
        { lastTime: -1, lines: [] as LogLine[] },
      ).lines;

      log.value = buffer;

      if (scroll && autoScroll.value) {
        scrollDown();
      }
    }, 500);

    async function download() {
      if (!repo?.value || !build.value || !proc.value) {
        throw new Error('The repository, build or proc was undefined');
      }
      let logs;
      try {
        downloadInProgress.value = true;
        logs = await apiClient.getLogs(repo.value.owner, repo.value.name, build.value.number, proc.value.pid);
      } catch (e) {
        notifications.notifyError(e, i18n.t('repo.build.log_download_error'));
        return;
      } finally {
        downloadInProgress.value = false;
      }
      const fileURL = window.URL.createObjectURL(
        new Blob([logs.map((line) => line.out).join('')], {
          type: 'text/plain',
        }),
      );
      const fileLink = document.createElement('a');

      fileLink.href = fileURL;
      fileLink.setAttribute(
        'download',
        `${repo.value.owner}-${repo.value.name}-${build.value.number}-${proc.value.name}.log`,
      );
      document.body.appendChild(fileLink);

      fileLink.click();
      document.body.removeChild(fileLink);
      window.URL.revokeObjectURL(fileURL);
    }

    async function loadLogs() {
      if (loadedProcSlug.value === procSlug.value) {
        return;
      }
      loadedProcSlug.value = procSlug.value;
      log.value = undefined;
      logBuffer.value = [];
      ansiUp.value = new AnsiUp();
      ansiUp.value.use_classes = true;

      if (!repo) {
        throw new Error('Unexpected: "repo" should be provided at this place');
      }

      if (stream.value) {
        stream.value.close();
      }

      if (!hasLogs.value || !proc.value) {
        return;
      }

      if (isProcFinished(proc.value)) {
        const logs = await apiClient.getLogs(repo.value.owner, repo.value.name, build.value.number, proc.value.pid);
        logs?.forEach((line) => writeLog({ index: line.pos, text: line.out, time: line.time }));
        flushLogs(false);
      }

      if (isProcRunning(proc.value)) {
        // load stream of parent process (which receives all child processes logs)
        // TODO: change stream to only send data of single child process
        stream.value = apiClient.streamLogs(
          repo.value.owner,
          repo.value.name,
          build.value.number,
          proc.value.ppid,
          (line) => {
            if (line?.proc !== proc.value?.name) {
              return;
            }
            writeLog({ index: line.pos, text: line.out, time: line.time });
            flushLogs(true);
          },
        );
      }
    }

    onMounted(async () => {
      loadLogs();
    });

    watch(procSlug, () => {
      loadLogs();
    });

    watch(proc, (oldProc, newProc) => {
      if (oldProc && oldProc.name === newProc?.name && oldProc?.end_time !== newProc?.end_time) {
        if (autoScroll.value) {
          scrollDown();
        }
      }
    });

    return { consoleElement, proc, log, loadedLogs, hasLogs, formatTime, showActions, download, downloadInProgress };
  },
});
</script>

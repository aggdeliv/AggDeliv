import os
os.environ['CUDA_VISIBLE_DEVICES'] = '-1'
import numpy as np
import multiprocessing as mp
import tensorflow.compat.v1 as tf
import a3c as model
import queue

# A_DIM = 6
# S_INFO = A_DIM + 1
# S_LEN = 8  # take how many frames in the past
ACTOR_LR_RATE = 0.001
CRITIC_LR_RATE = 0.01
TRAIN_SEQ_LEN = 8
# DEFAULT = 0
RAND_RANGE = 1000
GRADIENT_BATCH_SIZE = 8
MODEL_SAVE_INTERVAL = 100
SUMMARY_DIR = './results'
MODEL_DIR = './models'
NN_MODEL = './models/nn_model.ckpt'


def agent(agg_thp_queue, smp_queue, thp_queues, prob_queues, lock):
    if not os.path.exists(SUMMARY_DIR):
        os.makedirs(SUMMARY_DIR)
    if not os.path.exists(MODEL_DIR):
        os.makedirs(MODEL_DIR)
    with tf.Session() as sess:
        A_DIM = len(thp_queues)
        S_INFO = A_DIM
        S_LEN = smp_queue.get()
        actor = model.ActorNetwork(sess,state_dim=[S_INFO, S_LEN], action_dim=A_DIM, learning_rate=ACTOR_LR_RATE)
        critic = model.CriticNetwork(sess, state_dim=[S_INFO, S_LEN], learning_rate=CRITIC_LR_RATE)
        summary_ops, summary_vars = model.build_summaries()
        writer = tf.summary.FileWriter(SUMMARY_DIR, sess.graph)  # training monitor
        sess.run(tf.global_variables_initializer())
        saver = tf.train.Saver()

        # restore neural net parameters
        nn_model = NN_MODEL
        if os.path.exists(nn_model):
        # if nn_model is not None:  # nn_model is the path to file
            saver.restore(sess, nn_model)
            print("Model restored.")

        epoch = 0
        # action_vec = np.zeros(A_DIM,dtype=float)

        s_batch = [np.zeros((S_INFO, S_LEN))]
        a_batch = [np.zeros(A_DIM)]
        r_batch = []
        # thp_arr = []
        thp_arr = [[] for p in range(A_DIM)]
        entropy_record = []
        actor_gradient_batch = []
        critic_gradient_batch = []
        cur_agg_thp = 0.0
        smp_num = 0
        first_batch = True

        while True:
            lock.acquire()
            for i in range(A_DIM):
                while not thp_queues[i].empty():
                    if len(thp_arr[i]) == S_LEN:
                        del thp_arr[i][0]
                    thp_arr[i].append(thp_queues[i].get())
            lock.release()

            repeat = False
            for i in range(A_DIM):
                if len(thp_arr[i]) == 0:
                    repeat = True
                    break
            if repeat:
                continue

            agg_thp = agg_thp_queue.get()

            avg_thp = [0.0 for q in range(A_DIM)]
            sum_avg_thp = 0.0
            for i in range(A_DIM):
                for j in range(len(thp_arr[i])):
                    if thp_arr[i][j] != 0.0:
                        avg_thp[i] += 1 / thp_arr[i][j]

                if avg_thp[i] != 0.0:
                    avg_thp[i] = len(thp_arr[i]) / avg_thp[i]
                sum_avg_thp += avg_thp[i]

            for i in range(A_DIM):
                prob_queues[i].put(avg_thp[i] / sum_avg_thp)

            # if agg_thp == 0.0 or cur_agg_thp == 0.0:
            #     latest_thps = np.array(thp_arr, dtype=float)[:, -1]
            #     # if np.sum(latest_thps) == 0:
            #     #     continue
            #     action_vec = latest_thps / np.sum(latest_thps)
            #     # a_batch.append(action_vec)
            #     # action_vec = default_probs
            #     for i in range(A_DIM):
            #         prob_queues[i].put(action_vec[i])
            #     # del thp_arr[:]
            #     cur_agg_thp = agg_thp
            #     continue
            #
            # # reward = (agg_thp - cur_agg_thp) ** 2
            # # cur_agg_thp = agg_thp
            # reward = agg_thp
            # r_batch.append(reward)
            #
            # # retrieve previous state
            # # if len(s_batch) == 0:
            # #     state = [np.zeros((S_INFO, S_LEN))]
            # # else:
            # state = np.array(s_batch[-1], copy=True)
            #
            # # dequeue history record
            # # state = np.roll(state, -1, axis=1)
            # # state[0] = np.roll(state[0], -1)
            # # state[0, -1] = float(chunk_size) / float(tran_delay)
            #
            # for i in range(A_DIM):
            #     # print('len:' + str(len(thp_arr[i])))
            #     if len(thp_arr[i]) < S_LEN:
            #         state[i] = np.roll(state[i], -len(thp_arr[i]))
            #         state[i, -len(thp_arr[i]):] = np.array(thp_arr[i])
            #     else:
            #         state[i, :] = np.array(thp_arr[i])
            #     # state[i+1, -1] = np.array(thp_arr[i])
            #
            # # state[1, :A_DIM] = np.array(thp_arr)
            #
            # action_prob = actor.predict(np.reshape(state, (1, S_INFO, S_LEN)))[0]
            # # link_choice = action_prob.argmax()
            #
            # entropy_record.append(model.compute_entropy(action_prob))
            #
            # if len(r_batch) >= TRAIN_SEQ_LEN:  # do training once
            #     if first_batch:
            #         del s_batch[0]
            #         del a_batch[0]
            #         del r_batch[0]
            #         first_batch = False
            #     actor_gradient, critic_gradient, td_batch = \
            #         model.compute_gradients(s_batch=np.stack(s_batch, axis=0),  # ignore the first chuck
            #                               a_batch=np.vstack(a_batch),  # since we don't have the
            #                               r_batch=np.vstack(r_batch),  # control over it
            #                               terminal=False, actor=actor, critic=critic)
            #     td_loss = np.mean(td_batch)
            #     actor_gradient_batch.append(actor_gradient)
            #     critic_gradient_batch.append(critic_gradient)
            #
            #     print("====")
            #     print("Epoch", epoch)
            #     print("TD_loss", td_loss, "Avg_reward", np.mean(r_batch), "Avg_entropy", np.mean(entropy_record))
            #     print("====")
            #
            #     summary_str = sess.run(summary_ops, feed_dict={
            #         summary_vars[0]: td_loss,
            #         summary_vars[1]: np.mean(r_batch),
            #         summary_vars[2]: np.mean(entropy_record)
            #     })
            #
            #     writer.add_summary(summary_str, epoch)
            #     writer.flush()
            #
            #     entropy_record = []
            #     if len(actor_gradient_batch) >= GRADIENT_BATCH_SIZE:
            #         assert len(actor_gradient_batch) == len(critic_gradient_batch)
            #         for i in range(len(actor_gradient_batch)):
            #             actor.apply_gradients(actor_gradient_batch[i])
            #             critic.apply_gradients(critic_gradient_batch[i])
            #
            #         actor_gradient_batch = []
            #         critic_gradient_batch = []
            #
            #         epoch += 1
            #         if epoch % MODEL_SAVE_INTERVAL == 0:
            #             # Save the neural net parameters to disk.
            #             save_path = saver.save(sess, MODEL_DIR + "/nn_model_ep_" + str(epoch) + ".ckpt")
            #             saver.save(sess, nn_model)
            #             print("Model saved in file: %s" % save_path)
            #
            #     del s_batch[:]
            #     del a_batch[:]
            #     del r_batch[:]
            #
            # s_batch.append(state)
            # action_vec = np.array(action_prob)
            # a_batch.append(action_vec)
            # # del thp_arr[:]
            # for i in range(A_DIM):
            #     prob_queues[i].put(action_vec[i])

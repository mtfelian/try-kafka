# Kafka Hello World

This application was my "Kafka Hello World". 

It initializes local Kafka cluster with 2 topics: for tasks and for results. 

The client side accepts a command from stdin and sends it to the tasks topic, concurrently listening for the results topics.

Server side listens for the tasks topic, calculates expression (task is a simple arithmetical expression) and sends it to the results topic. 

